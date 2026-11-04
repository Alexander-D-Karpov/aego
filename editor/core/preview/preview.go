package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"image"
	stddraw "image/draw"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/audio/codec"
	_ "aego/audio/codec/vorbis"
	_ "aego/audio/codec/wav"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/editor/core/shape"
	"aego/editor/core/tasks"
	"aego/engine"
	"aego/engine/i18n"
	"aego/engine/problems"
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
	salt      = "gallery.v2"
	lineCount = 30
	CoverFile = "preview.png"
)

type Deps struct {
	Problems   *problems.Service
	Docs       *document.Manager
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
	dispose        func()
	motion         Motion
	hoverRequested bool
	motionLoaded   bool
	role           asset.TextureRole
	id             ids.AssetID
	rev            uint64
	key            string
	state          state
	tex            gpu.TextureHandle
	region         texture.Region
	lines          []string
	task           *tasks.Task
	content        pixels
	reason         string
	draft          uint64
}

type Service struct {
	sources        map[ids.AssetID]sourceFile
	fontOnce       sync.Once
	font           image.Image
	chars          map[rune]glyph
	issueRev       uint64
	disconnect     func()
	d              Deps
	dir            string
	items          map[ids.AssetID]*item
	peaks          map[peakKey]*peakItem
	frame          int64
	gpuOK          bool
	pool           *rt.Pool
	coverKey       string
	handlers       map[asset.ArtifactKind]Handler
	presetHandlers []Handler
	draft          uint64
}

func New(d Deps) *Service {
	s := &Service{d: d, items: map[ids.AssetID]*item{}, dir: filepath.Join(d.CacheRoot, CacheDir)}
	if d.Engine != nil && d.GPU != nil && d.Renderer != nil {
		s.gpuOK = !strings.Contains(strings.ToLower(d.GPU.Caps().Renderer), "null")
	}
	if d.Tasks != nil && d.CacheRoot != "" {
		legacy := filepath.Join(d.CacheRoot, legacyDir)
		d.Tasks.Work(tasks.Spec{Title: i18n.Tr("editor.preview.remove_old_thumbnails", "Remove old thumbnails"), Kind: "preview"}, func(*tasks.Task) error {
			return os.RemoveAll(legacy)
		})
	}
	if d.Problems != nil {
		s.disconnect = d.Problems.Add(problems.Source{Revision: func() uint64 { return s.issueRev }, Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
			for _, it := range s.items {
				if it.reason != "" {
					e, _ := s.entry(it.id)
					dst = append(dst, problems.Diagnostic{Owner: "editor.preview", Code: "THUMB001", Severity: problems.Warning, Message: it.reason, Subject: problems.Subject{Kind: problems.SubjectAsset, Asset: it.id, File: e.Source}})
				}
			}
			return dst
		}})
	}
	return s
}

func (s *Service) Begin() {
	s.frame++
	s.draft = s.documentEpoch()
	if s.d.Cover == nil || !s.gpuOK {
		return
	}
	if id := s.d.Cover(); !id.IsZero() {
		s.ByID(id)
	}
}

func Image(k asset.ArtifactKind) bool {
	return k == asset.ArtAtlasPage || k == asset.ArtTexture || k == asset.ArtSpriteRegion || k == asset.ArtAudio
}

func Rendered(k asset.ArtifactKind) bool {
	return k == asset.ArtScene || k == asset.ArtPrefab
}

func (s *Service) entry(id ids.AssetID) (asset.Entry, bool) {
	if f, ok := s.sources[id]; ok {
		return asset.Entry{ID: id, Kind: SourceKind, Source: f.info.Path, Name: f.info.Name}, true
	}
	if b, ok := asset.LookupBuiltin(id); ok {
		return asset.Entry{ID: id, Kind: b.Kind, Name: b.Name, Source: b.Name}, true
	}
	if s.d.Pipeline == nil || id.IsZero() {
		return asset.Entry{}, false
	}
	return s.d.Pipeline.Entry(id)
}

func (s *Service) Key(e asset.Entry) (string, bool) {
	art, ok := e.Artifact(e.Kind)
	if (!ok || art.Hash == "") && e.Kind != asset.ArtScript && e.Source == "" && !asset.IsBuiltin(e.ID) {
		return "", false
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s:%d:%s:%s", salt, Size, s.handlers[e.Kind].Version, e.Kind)
	if s.handlers[e.Kind].Render != nil && !s.gpuOK {
		fmt.Fprint(h, "|no-gpu")
	}
	seen := map[ids.AssetID]bool{}
	var visit func(asset.Entry)
	visit = func(x asset.Entry) {
		if seen[x.ID] {
			return
		}
		seen[x.ID] = true
		fmt.Fprintf(h, "|%s:%s:%s:%s", x.ID.String(), x.Source, x.Kind, x.Error)
		if asset.IsBuiltin(x.ID) {
			var data []byte
			switch x.Kind {
			case asset.ArtShader:
				data, _ = asset.BuiltinShader(x.ID)
			case asset.ArtMaterial:
				data, _ = asset.BuiltinMaterial(x.ID)
			}
			fmt.Fprintf(h, "|%x", sha256.Sum256(data))
		}
		if len(x.Artifacts) == 0 && !asset.IsBuiltin(x.ID) && s.d.Pipeline != nil {
			if st, err := os.Stat(s.d.Pipeline.Abs(x.Source)); err == nil {
				fmt.Fprintf(h, "|%d:%d", st.ModTime().UnixNano(), st.Size())
			}
		}
		for _, a := range x.Artifacts {
			fmt.Fprintf(h, "|%s:%s", a.Kind, a.Hash)
		}
		if doc := s.d.Docs; doc != nil {
			if a := doc.AssetDocument(x.ID); a != nil && a.Binding.PreviewSource != nil {
				fmt.Fprintf(h, "|%x", sha256.Sum256(a.Binding.PreviewSource()))
			}
			for _, name := range []string{x.Source, x.Source + ".meta"} {
				if d := doc.Script(name); d != nil && d.Dirty() {
					fmt.Fprintf(h, "|%x", sha256.Sum256(d.Code.Bytes()))
				}
			}
			if d := doc.Find(x.Source); d != nil && d.Dirty() {
				fmt.Fprintf(h, "|%x", sha256.Sum256(d.PreviewBytes()))
			}
		}
		deps := slices.Clone(x.Deps)
		slices.SortFunc(deps, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
		for _, dep := range deps {
			if child, ok := s.entry(dep); ok {
				visit(child)
			} else {
				fmt.Fprintf(h, "|%s", dep.String())
			}
		}
	}
	visit(e)
	if s.handlers[e.Kind].Render != nil && s.d.Docs != nil {
		for _, d := range s.d.Docs.Scripts() {
			if d.Dirty() && d.Code.GLSL {
				fmt.Fprintf(h, "|%s:%x", d.Path, sha256.Sum256(d.Code.Bytes()))
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func (s *Service) documentEpoch() uint64 {
	if s.d.Docs == nil {
		return 0
	}
	var n uint64
	mix := func(id, rev uint64) { n = (n ^ id) * 1099511628211; n = (n ^ rev) * 1099511628211 }
	for _, d := range s.d.Docs.Scripts() {
		if d.Dirty() {
			mix(uint64(d.ID), d.Code.Rev())
		}
	}
	for _, a := range s.d.Docs.AssetDocuments() {
		if a.Binding.PreviewRevision != nil {
			mix(uint64(a.ID), a.Binding.PreviewRevision())
		}
	}
	for _, d := range s.d.Docs.Docs() {
		if d.Dirty() {
			mix(uint64(d.ID), d.Scene().Revision())
		}
	}
	return n
}

func (s *Service) ByID(id ids.AssetID) (texture.Region, bool) {
	if reg, ok := s.builtin(id); ok {
		return reg, true
	}
	e, ok := s.entry(id)
	if !ok {
		return texture.Region{}, false
	}
	return s.Get(pipeline.AssetInfo{ID: id, Kind: e.Kind, Path: e.Source})
}

func (s *Service) Get(info pipeline.AssetInfo) (texture.Region, bool) {

	it := s.ensure(info)
	if it == nil {
		return texture.Region{}, false
	}
	return it.region, it.state == ready && !it.region.IsZero()
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

func (s *Service) ensure(info pipeline.AssetInfo) *item { return s.request(info, s.frame*2+1) }
func (s *Service) Prefetch(info pipeline.AssetInfo)     { s.request(info, s.frame*2) }
func (s *Service) request(info pipeline.AssetInfo, priority int64) *item {
	rev := s.revision()
	if it := s.items[info.ID]; it != nil && it.rev == rev && it.draft == s.draft && it.key != "" && it.key != builtinKey {
		if it.state == pending && it.task != nil {
			it.task.SetPriority(max(it.task.Priority, priority))
		}
		return it
	}
	e, ok := s.entry(info.ID)
	if !ok && info.Path != "" {
		e = asset.Entry{ID: info.ID, Source: info.Path, Name: info.Name, Kind: info.Kind, Error: info.Error}
		ok = true
	}
	if !ok || s.d.Tasks == nil {
		return nil
	}
	if info.Error != "" {
		e.Error = info.Error
	}
	key, ok := s.Key(e)
	if !ok {
		return nil
	}
	it := s.items[info.ID]
	if it != nil && it.key == key {
		it.rev, it.draft = rev, s.draft
		if it.state == pending && it.task != nil {
			it.task.SetPriority(max(it.task.Priority, priority))
		}
		return it
	}
	if it != nil {
		it.release()
		if it.task != nil {
			it.task.Cancel()
		}
		if !it.tex.IsZero() {
			s.d.GPU.DestroyTexture(it.tex)
		}
	}
	s.issueRev++
	it = &item{id: info.ID}
	s.items[info.ID] = it
	it.key, it.state, it.rev, it.draft, it.reason = key, pending, rev, s.draft, ""
	spec := tasks.Spec{Title: i18n.Tr("editor.preview.preview_name", "Preview {name}", i18n.Str("name", filepath.Base(e.Name))), Kind: "preview", Priority: priority}
	if info.Error != "" {
		e.Error = info.Error
	}
	switch {
	case e.Error != "" || e.Missing:
		s.startCard(e, it, spec, true)
	case s.handlers[e.Kind].Render != nil && s.gpuOK:
		s.startCustom(e, it, spec, s.handlers[e.Kind])
	case Rendered(e.Kind) && s.gpuOK:
		s.startScene(e, it, spec)
	case Image(e.Kind):
		s.startImage(e, it, spec)
	default:
		s.startCard(e, it, spec, false)
	}
	return it
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
	var role asset.TextureRole
	it.task = s.d.Tasks.Work(spec, func(t *tasks.Task) error {
		if s.d.Pipeline != nil && (e.Kind == asset.ArtTexture || e.Kind == asset.ArtSpriteRegion) {
			if info, ok := s.d.Pipeline.TextureSourceInfo(e.ID, nil); ok {
				role = info.Role
			}
		}
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
	}).Then(func(t *tasks.Task) {
		if s.items[it.id] != it || it.key != key {
			return
		}
		it.role = role
		if t.Err() != nil {
			e.Error = t.Err().Error()
			s.startCard(e, it, spec, true)
			return
		}
		s.finish(it, key, out, t)
	})
}

func (s *Service) finish(it *item, key string, p pixels, t *tasks.Task) {
	if s.items[it.id] != it || it.key != key {
		return
	}
	it.task = nil
	s.issueRev++
	if t.State() != tasks.Done || len(p.buf) == 0 {
		it.state = failed
		if t.Err() != nil {
			it.reason = t.Err().Error()
		}
		return
	}
	it.content = p
	it.state = ready
	if !s.gpuOK {
		return
	}
	data := &texture.Data{Width: p.w, Height: p.h, Format: gpu.TexSRGBA8, Pixels: slices.Clone(p.buf)}
	tex, err := s.d.Renderer.Textures().Upload(data, texture.UploadOptions{Label: "editor.preview"})
	if err != nil {
		it.reason = err.Error()
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
	s.d.Tasks.Work(tasks.Spec{Title: i18n.Tr("editor.preview.project_preview", "Project preview"), Kind: "preview"}, func(*tasks.Task) error {
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
	tmp, err := os.CreateTemp(s.dir, "thumb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(buf.Bytes())
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), s.cachePath(key))
}

func (s *Service) buildImage(e asset.Entry) (pixels, error) {
	if buf, ok := asset.BuiltinAudio(e.ID); ok {
		return s.waveform(buf)
	}
	if tt, ok := asset.BuiltinTexture(e.ID); ok {
		small, w, h := downscale(tt.Pixels, tt.Width, tt.Height, Size)
		return pixels{buf: small, w: w, h: h}, nil
	}

	files := s.d.Pipeline.Artifacts()
	art, _ := e.Artifact(e.Kind)
	data, err := files.ReadFile(art.Path)
	if err != nil {
		return pixels{}, err
	}
	var buf []byte
	var w, h int
	var img *asset.TextureArtifact
	switch e.Kind {
	case asset.ArtTexture, asset.ArtAtlasPage:
		ta, err := asset.DecodeTexture(data)
		if err != nil {
			return pixels{}, err
		}
		img = ta
		buf, w, h = ta.Pixels, ta.Width, ta.Height
	case asset.ArtSpriteRegion:
		ra, err := asset.DecodeRegion(data)
		if err != nil {
			return pixels{}, err
		}
		// This runs on the thumbnail worker. Editor-owned s.sources may be
		// rebuilt by RefreshSources on the main thread while we decode.
		pa, ok := s.d.Pipeline.PrimaryArtifact(ra.Page)
		if !ok || (pa.Kind != asset.ArtAtlasPage && pa.Kind != asset.ArtTexture) {
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
		if ra.PageW != ta.Width || ra.PageH != ta.Height {
			return pixels{}, errors.New("atlas region no longer matches its page")
		}
		img = ta
		buf, w, h = crop(ta.Pixels, ta.Width, ra.X, ra.Y, ra.W, ra.H), ra.W, ra.H
	case asset.ArtAudio:
		return s.waveform(data)
	default:
		return pixels{}, errs.New(errs.AssetBadArtifact, "asset has no image")
	}
	small, sw, sh := downscale(buf, w, h, Size)
	switch img.Semantic {
	case asset.TextureColor:
		straight(small, img.Format == gpu.TexSRGBA8)
	case asset.TextureEmissionRGB:
		if img.Format == gpu.TexRGBA8 {
			encodeRGB(small)
		}
	default:
		for i := 3; i < len(small); i += 4 {
			small[i] = 255
		}
	}
	return pixels{buf: small, w: sw, h: sh}, nil
}

func (s *Service) waveform(data []byte) (pixels, error) {
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
	im := &image.NRGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	if a.Rate > 0 {
		s.label(im, 3, h-14, fmt.Sprintf("%.2f s", float64(a.Frames)/float64(a.Rate)), ink)
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
	it.release()
	if it.task != nil {
		it.task.Cancel()
	}
	if !it.tex.IsZero() {
		s.d.GPU.DestroyTexture(it.tex)
	}
	delete(s.items, id)
	s.issueRev++
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
	if s.disconnect != nil {
		s.disconnect()
		s.disconnect = nil
	}
	for id := range s.items {
		s.Forget(id)
	}
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
}

// builtin shows an engine texture, such as a runner frame, which has no entry in the project's index.
func (s *Service) builtin(id ids.AssetID) (texture.Region, bool) {
	if it := s.items[id]; it != nil && it.key == builtinKey {
		return it.region, it.state == ready
	}
	data, ok := asset.BuiltinTexture(id)
	if !ok || !s.gpuOK {
		return texture.Region{}, false
	}
	it := &item{id: id, key: builtinKey, state: failed}
	s.items[id] = it
	tex, err := s.d.Renderer.Textures().Upload(data, texture.UploadOptions{Label: "editor.preview"})
	if err != nil {
		return texture.Region{}, false
	}
	it.tex, it.region, it.state = tex, texture.Whole(tex, data.Width, data.Height), ready
	return it.region, true
}

const builtinKey = "builtin"

// revision changes whenever the pipeline imports, so a key is worked out again only then.
func (s *Service) revision() uint64 {
	if s.d.Pipeline == nil {
		return 0
	}
	return s.d.Pipeline.Revision()
}

func (it *item) release() {
	if it.dispose != nil {
		it.dispose()
		it.dispose = nil
	}
}
