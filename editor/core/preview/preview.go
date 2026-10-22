package preview

import (
	"bytes"
	"encoding/hex"
	"hash/fnv"
	"image"
	stddraw "image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	_ "aego/audio/codec/wav"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/editor/core/tasks"
	"aego/engine"
	"aego/gpu"
	"aego/render"
	"aego/render/rt"
	"aego/render/texture"
	"aego/scene"
)

const (
	Size      = 128
	CacheDir  = "thumbs"
	legacyDir = "thumbnails"
	salt      = "preview.v1"
	lineCount = 30
	CoverFile = "preview.png"
)

type Deps struct {
	Engine     *engine.Engine
	Pipeline   *pipeline.Pipeline
	Renderer   *render.Renderer
	GPU        gpu.Device
	Types      *scene.Types
	Resolver   asset.Resolver
	Shapes     *shape.Registry
	Tasks      *tasks.Queue
	CacheRoot  string
	Background mathx.Color
	Cover      func() ids.AssetID
	Reference  func() mathx.Vec2
}

type state uint8

const (
	pending state = iota
	ready
	failed
)

type item struct {
	id     ids.AssetID
	key    string
	state  state
	tex    gpu.TextureHandle
	region texture.Region
	lines  []string
	task   *tasks.Task
}

type Service struct {
	d        Deps
	dir      string
	items    map[ids.AssetID]*item
	peaks    map[peakKey]*peakItem
	frame    int64
	gpuOK    bool
	pool     *rt.Pool
	coverKey string
}

func New(d Deps) *Service {
	s := &Service{d: d, items: map[ids.AssetID]*item{}, dir: filepath.Join(d.CacheRoot, CacheDir)}
	if d.Engine != nil && d.GPU != nil && d.Renderer != nil {
		s.gpuOK = !strings.Contains(strings.ToLower(d.GPU.Caps().Renderer), "null")
	}
	if d.Tasks != nil && d.CacheRoot != "" {
		legacy := filepath.Join(d.CacheRoot, legacyDir)
		d.Tasks.Work(tasks.Spec{Title: "Remove old thumbnails", Kind: "preview"}, func(*tasks.Task) error {
			return os.RemoveAll(legacy)
		})
	}
	return s
}

func (s *Service) Begin() {
	s.frame++
	if s.d.Cover == nil || !s.gpuOK {
		return
	}
	if id := s.d.Cover(); !id.IsZero() {
		s.ByID(id)
	}
}

func Image(k asset.ArtifactKind) bool {
	return k == asset.ArtTexture || k == asset.ArtSpriteRegion || k == asset.ArtAudio
}

func Rendered(k asset.ArtifactKind) bool {
	return k == asset.ArtScene || k == asset.ArtPrefab
}

func (s *Service) entry(id ids.AssetID) (asset.Entry, bool) {
	if s.d.Pipeline == nil || id.IsZero() {
		return asset.Entry{}, false
	}
	return s.d.Pipeline.Entry(id)
}

func (s *Service) Key(e asset.Entry) (string, bool) {
	art, ok := e.Artifact(e.Kind)
	if (!ok || art.Hash == "") && e.Kind == asset.ArtScript {
		return s.scriptKey(e)
	}
	if !ok || art.Hash == "" {
		return "", false
	}
	h := fnv.New64a()
	h.Write([]byte(salt))
	h.Write([]byte(art.Hash))
	if Rendered(e.Kind) {
		deps := slices.Clone(e.Deps)
		slices.SortFunc(deps, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
		for _, d := range deps {
			if de, ok := s.entry(d); ok {
				if da, ok := de.Artifact(de.Kind); ok {
					h.Write([]byte(da.Hash))
				}
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func (s *Service) scriptKey(e asset.Entry) (string, bool) {
	h := fnv.New64a()
	h.Write([]byte(salt))
	h.Write([]byte(e.Source))
	for _, d := range e.Deps {
		if de, ok := s.entry(d); ok {
			for _, a := range de.Artifacts {
				h.Write([]byte(a.Hash))
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func (s *Service) ByID(id ids.AssetID) (texture.Region, bool) {
	e, ok := s.entry(id)
	if !ok {
		return texture.Region{}, false
	}
	return s.Get(pipeline.AssetInfo{ID: id, Kind: e.Kind, Path: e.Source})
}

func (s *Service) Get(info pipeline.AssetInfo) (texture.Region, bool) {
	if !Image(info.Kind) && !(Rendered(info.Kind) && s.gpuOK) {
		return texture.Region{}, false
	}
	it := s.ensure(info)
	if it == nil {
		return texture.Region{}, false
	}
	return it.region, !it.region.IsZero()
}

func (s *Service) Missing(info pipeline.AssetInfo) bool {
	switch {
	case Image(info.Kind):
	case Rendered(info.Kind):
		if !s.gpuOK {
			return true
		}
	default:
		return true
	}
	it := s.items[info.ID]
	return it != nil && it.state == failed && it.region.IsZero()
}

func (s *Service) Lines(info pipeline.AssetInfo) []string {
	if info.Kind != asset.ArtScript {
		return nil
	}
	if it := s.ensure(info); it != nil {
		return it.lines
	}
	return nil
}

func (s *Service) ensure(info pipeline.AssetInfo) *item {
	e, ok := s.entry(info.ID)
	if !ok || s.d.Tasks == nil {
		return nil
	}
	key, ok := s.Key(e)
	if !ok {
		return nil
	}
	it := s.items[info.ID]
	if it != nil && it.key == key {
		if it.state == pending && it.task != nil {
			it.task.SetPriority(s.frame)
		}
		return it
	}
	if it == nil {
		it = &item{id: info.ID}
		s.items[info.ID] = it
	}
	if it.task != nil {
		it.task.Cancel()
	}
	it.key, it.state = key, pending
	spec := tasks.Spec{Title: "Preview " + filepath.Base(e.Name), Kind: "preview", Priority: s.frame}
	switch {
	case info.Kind == asset.ArtScript:
		s.startLines(info, it, spec)
	case Image(e.Kind):
		s.startImage(e, it, spec)
	default:
		s.startScene(e, it, spec)
	}
	return it
}

func (s *Service) startLines(info pipeline.AssetInfo, it *item, spec tasks.Spec) {
	key, abs := it.key, s.d.Pipeline.Abs(info.Path)
	var lines []string
	it.task = s.d.Tasks.Work(spec, func(*tasks.Task) error {
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		lines = firstLines(string(data), lineCount)
		return nil
	}).Then(func(t *tasks.Task) {
		if it.key != key {
			return
		}
		it.task = nil
		if t.State() != tasks.Done {
			it.state = failed
			return
		}
		it.lines, it.state = lines, ready
	})
}

func firstLines(src string, n int) []string {
	var out []string
	imports := false
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimRight(strings.ReplaceAll(l, "\t", "  "), " \r")
		t := strings.TrimSpace(l)
		switch {
		case imports:
			imports = t != ")"
			continue
		case strings.HasPrefix(t, "import ("):
			imports = true
			continue
		case t == "" || strings.HasPrefix(l, "package ") || strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "//"):
			continue
		}
		out = append(out, l)
		if len(out) == n {
			break
		}
	}
	return out
}

type pixels struct {
	buf  []byte
	w, h int
}

func (s *Service) startImage(e asset.Entry, it *item, spec tasks.Spec) {
	key := it.key
	var out pixels
	it.task = s.d.Tasks.Work(spec, func(t *tasks.Task) error {
		if p, err := s.load(key); err == nil {
			out = p
			return nil
		}
		p, err := s.buildImage(e)
		if err != nil {
			return err
		}
		out = p
		return s.save(key, p)
	}).Then(func(t *tasks.Task) { s.finish(it, key, out, t) })
}

func (s *Service) finish(it *item, key string, p pixels, t *tasks.Task) {
	if it.key != key {
		return
	}
	it.task = nil
	if t.State() != tasks.Done || len(p.buf) == 0 {
		it.state = failed
		return
	}
	data := &texture.Data{Width: p.w, Height: p.h, Format: gpu.TexSRGBA8, Pixels: p.buf, Premultiplied: true}
	tex, err := s.d.Renderer.Textures().Upload(data, texture.UploadOptions{Label: "editor.preview"})
	if err != nil {
		it.state = failed
		return
	}
	if !it.tex.IsZero() {
		s.d.GPU.DestroyTexture(it.tex)
	}
	it.tex, it.region, it.state = tex, texture.Whole(tex, p.w, p.h), ready
	s.cover(it, p)
}

func (s *Service) cover(it *item, p pixels) {
	if s.d.Cover == nil || s.d.CacheRoot == "" || it.id != s.d.Cover() || it.key == s.coverKey {
		return
	}
	s.coverKey = it.key
	dst := filepath.Join(s.d.CacheRoot, CoverFile)
	s.d.Tasks.Work(tasks.Spec{Title: "Project preview", Kind: "preview"}, func(*tasks.Task) error {
		var buf bytes.Buffer
		img := &image.NRGBA{Pix: p.buf, Stride: p.w * 4, Rect: image.Rect(0, 0, p.w, p.h)}
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		tmp := dst + ".tmp"
		if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, dst)
	})
}

func (s *Service) cachePath(key string) string {
	return filepath.Join(s.dir, key+".png")
}

func (s *Service) load(key string) (pixels, error) {
	data, err := os.ReadFile(s.cachePath(key))
	if err != nil {
		return pixels{}, err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return pixels{}, err
	}
	n, ok := img.(*image.NRGBA)
	if !ok {
		n = image.NewNRGBA(img.Bounds())
		stddraw.Draw(n, n.Bounds(), img, img.Bounds().Min, stddraw.Src)
	}
	b := n.Bounds()
	return pixels{buf: n.Pix, w: b.Dx(), h: b.Dy()}, nil
}

func (s *Service) save(key string, p pixels) error {
	var buf bytes.Buffer
	img := &image.NRGBA{Pix: p.buf, Stride: p.w * 4, Rect: image.Rect(0, 0, p.w, p.h)}
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp := s.cachePath(key) + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.cachePath(key))
}

func (s *Service) buildImage(e asset.Entry) (pixels, error) {
	files := s.d.Pipeline.Artifacts()
	art, _ := e.Artifact(e.Kind)
	data, err := files.ReadFile(art.Path)
	if err != nil {
		return pixels{}, err
	}
	var buf []byte
	var w, h int
	switch e.Kind {
	case asset.ArtTexture:
		ta, err := asset.DecodeTexture(data)
		if err != nil {
			return pixels{}, err
		}
		buf, w, h = ta.Pixels, ta.Width, ta.Height
	case asset.ArtSpriteRegion:
		ra, err := asset.DecodeRegion(data)
		if err != nil {
			return pixels{}, err
		}
		pe, ok := s.entry(ra.Page)
		if !ok {
			return pixels{}, errs.New(errs.AssetNotFound, "atlas page is missing")
		}
		pa, ok := pe.Artifact(pe.Kind)
		if !ok {
			return pixels{}, errs.New(errs.AssetNotFound, "atlas page artifact is missing")
		}
		pdata, err := files.ReadFile(pa.Path)
		if err != nil {
			return pixels{}, err
		}
		ta, err := asset.DecodeTexture(pdata)
		if err != nil {
			return pixels{}, err
		}
		buf, w, h = crop(ta.Pixels, ta.Width, ra.X, ra.Y, ra.W, ra.H), ra.W, ra.H
	case asset.ArtAudio:
		return waveform(data)
	default:
		return pixels{}, errs.New(errs.AssetBadArtifact, "asset has no image")
	}
	small, sw, sh := downscale(buf, w, h, Size)
	return pixels{buf: small, w: sw, h: sh}, nil
}

func waveform(data []byte) (pixels, error) {
	a, err := asset.DecodeAudio(data)
	if err != nil {
		return pixels{}, err
	}
	d, err := codec.Open(bytes.NewReader(a.Data), "")
	if err != nil {
		return pixels{}, err
	}
	defer d.Close()
	peaks, err := codec.Peaks(d, Size)
	if err != nil {
		return pixels{}, err
	}
	w, h := Size, Size/2
	buf := make([]byte, w*h*4)
	set := func(x, y int, r, g, b byte) {
		i := (y*w + x) * 4
		buf[i], buf[i+1], buf[i+2], buf[i+3] = r, g, b, 255
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			set(x, y, 0x1d, 0x20, 0x26)
		}
	}
	mid := h / 2
	for x, p := range peaks {
		amp := int(min(p, 1) * float32(mid-1))
		for y := mid - amp; y <= mid+amp; y++ {
			set(x, y, 0x6e, 0x9b, 0xf5)
		}
	}
	return pixels{buf: buf, w: w, h: h}, nil
}

func crop(src []byte, stride, x, y, w, h int) []byte {
	out := make([]byte, w*h*4)
	for row := 0; row < h; row++ {
		si := ((y+row)*stride + x) * 4
		copy(out[row*w*4:(row+1)*w*4], src[si:si+w*4])
	}
	return out
}

func downscale(src []byte, w, h, limit int) ([]byte, int, int) {
	if w <= limit && h <= limit {
		return append([]byte(nil), src[:w*h*4]...), w, h
	}
	f := float64(max(w, h)) / float64(limit)
	nw, nh := max(1, int(float64(w)/f)), max(1, int(float64(h)/f))
	out := make([]byte, nw*nh*4)
	for y := 0; y < nh; y++ {
		y0 := int(float64(y) * f)
		y1 := min(h, max(y0+1, int(float64(y+1)*f)))
		for x := 0; x < nw; x++ {
			x0 := int(float64(x) * f)
			x1 := min(w, max(x0+1, int(float64(x+1)*f)))
			var sum [4]int
			n := 0
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					i := (yy*w + xx) * 4
					sum[0] += int(src[i])
					sum[1] += int(src[i+1])
					sum[2] += int(src[i+2])
					sum[3] += int(src[i+3])
					n++
				}
			}
			o := (y*nw + x) * 4
			for c := 0; c < 4; c++ {
				out[o+c] = byte(sum[c] / n)
			}
		}
	}
	return out, nw, nh
}

func (s *Service) Forget(id ids.AssetID) {
	for k, pk := range s.peaks {
		if k.id != id {
			continue
		}
		if pk.task != nil {
			pk.task.Cancel()
		}
		delete(s.peaks, k)
	}
	it, ok := s.items[id]
	if !ok {
		return
	}
	if it.task != nil {
		it.task.Cancel()
	}
	if !it.tex.IsZero() {
		s.d.GPU.DestroyTexture(it.tex)
	}
	delete(s.items, id)
}

func (s *Service) Busy() bool {
	for _, it := range s.items {
		if it.state == pending {
			return true
		}
	}
	return false
}

func (s *Service) Close() {
	for id := range s.items {
		s.Forget(id)
	}
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
}
