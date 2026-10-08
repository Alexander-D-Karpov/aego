package assets

import (
	"os"
	"path/filepath"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/context"
	"aego/gpu"
	"aego/render/texture"
)

const (
	ThumbSize   = 64
	maxInflight = 4
)

type thumbState uint8

const (
	thumbPending thumbState = iota
	thumbReady
	thumbFailed
)

type thumb struct {
	hash   string
	state  thumbState
	tex    gpu.TextureHandle
	region texture.Region
}

type Thumbs struct {
	ctx      *context.Editor
	dir      string
	items    map[ids.AssetID]*thumb
	inflight int
}

func NewThumbs(ctx *context.Editor) *Thumbs {
	return &Thumbs{
		ctx:   ctx,
		dir:   filepath.Join(ctx.Project.CacheDir(), "thumbnails"),
		items: map[ids.AssetID]*thumb{},
	}
}

func (t *Thumbs) ByID(id ids.AssetID) (texture.Region, bool) {
	if t.ctx.Pipeline == nil || id.IsZero() {
		return texture.Region{}, false
	}
	e, ok := t.ctx.Pipeline.Entry(id)
	if !ok {
		return texture.Region{}, false
	}
	return t.Get(pipeline.AssetInfo{ID: id, Kind: e.Kind})
}

func (t *Thumbs) Get(info pipeline.AssetInfo) (texture.Region, bool) {
	if t.ctx.Pipeline == nil || (info.Kind != asset.ArtTexture && info.Kind != asset.ArtSpriteRegion) {
		return texture.Region{}, false
	}
	e, ok := t.ctx.Pipeline.Entry(info.ID)
	if !ok {
		return texture.Region{}, false
	}
	art, ok := e.Artifact(e.Kind)
	if !ok {
		return texture.Region{}, false
	}
	th := t.items[info.ID]
	if th != nil && th.hash == art.Hash {
		return th.region, th.state == thumbReady || !th.region.IsZero()
	}
	if t.inflight >= maxInflight {
		if th != nil && !th.region.IsZero() {
			return th.region, true
		}
		return texture.Region{}, false
	}
	if th == nil {
		th = &thumb{}
		t.items[info.ID] = th
	}
	th.hash = art.Hash
	th.state = thumbPending
	t.inflight++
	id, hash := info.ID, art.Hash
	jobs := t.ctx.Engine.Jobs()
	jobs.Submit(func() {
		pixels, w, h, err := t.build(e, hash)
		jobs.OnMain(func() { t.finish(id, hash, pixels, w, h, err) })
	})
	return th.region, !th.region.IsZero()
}

func (t *Thumbs) build(e asset.Entry, hash string) ([]byte, int, int, error) {
	cachePath := filepath.Join(t.dir, hash+".aetx")
	if data, err := os.ReadFile(cachePath); err == nil {
		if ta, err := asset.DecodeTexture(data); err == nil {
			return ta.Pixels, ta.Width, ta.Height, nil
		}
	}
	files := t.ctx.Pipeline.Artifacts()
	var pixels []byte
	var w, h int
	switch e.Kind {
	case asset.ArtTexture:
		art, _ := e.Artifact(asset.ArtTexture)
		data, err := files.ReadFile(art.Path)
		if err != nil {
			return nil, 0, 0, err
		}
		ta, err := asset.DecodeTexture(data)
		if err != nil {
			return nil, 0, 0, err
		}
		pixels, w, h = ta.Pixels, ta.Width, ta.Height
	case asset.ArtSpriteRegion:
		art, _ := e.Artifact(asset.ArtSpriteRegion)
		data, err := files.ReadFile(art.Path)
		if err != nil {
			return nil, 0, 0, err
		}
		ra, err := asset.DecodeRegion(data)
		if err != nil {
			return nil, 0, 0, err
		}
		pe, ok := t.ctx.Pipeline.Entry(ra.Page)
		if !ok {
			return nil, 0, 0, errs.New(errs.AssetNotFound, "atlas page is missing")
		}
		pa, ok := pe.Artifact(pe.Kind)
		if !ok {
			return nil, 0, 0, errs.New(errs.AssetNotFound, "atlas page artifact is missing")
		}
		pdata, err := files.ReadFile(pa.Path)
		if err != nil {
			return nil, 0, 0, err
		}
		ta, err := asset.DecodeTexture(pdata)
		if err != nil {
			return nil, 0, 0, err
		}
		pixels, w, h = crop(ta.Pixels, ta.Width, ra.X, ra.Y, ra.W, ra.H), ra.W, ra.H
	default:
		return nil, 0, 0, errs.New(errs.AssetBadArtifact, "asset has no image")
	}
	small, sw, sh := downscale(pixels, w, h, ThumbSize)
	enc := asset.EncodeTexture(&asset.TextureArtifact{Width: sw, Height: sh, Format: gpu.TexSRGBA8, Pixels: small})
	if err := os.MkdirAll(t.dir, 0o755); err == nil {
		os.WriteFile(cachePath, enc, 0o644)
	}
	return small, sw, sh, nil
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
	s := float64(max(w, h)) / float64(limit)
	nw, nh := max(1, int(float64(w)/s)), max(1, int(float64(h)/s))
	out := make([]byte, nw*nh*4)
	for y := 0; y < nh; y++ {
		y0 := int(float64(y) * s)
		y1 := min(h, max(y0+1, int(float64(y+1)*s)))
		for x := 0; x < nw; x++ {
			x0 := int(float64(x) * s)
			x1 := min(w, max(x0+1, int(float64(x+1)*s)))
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

func (t *Thumbs) finish(id ids.AssetID, hash string, pixels []byte, w, h int, err error) {
	t.inflight--
	th := t.items[id]
	if th == nil || th.hash != hash {
		return
	}
	if err != nil {
		th.state = thumbFailed
		return
	}
	data := &texture.Data{Width: w, Height: h, Format: gpu.TexSRGBA8, Pixels: pixels, Premultiplied: true}
	tex, err := t.ctx.Renderer.Textures().Upload(data, texture.UploadOptions{Label: "editor.thumb"})
	if err != nil {
		th.state = thumbFailed
		return
	}
	if !th.tex.IsZero() {
		t.ctx.Engine.GPU().DestroyTexture(th.tex)
	}
	th.tex = tex
	th.region = texture.Whole(tex, w, h)
	th.state = thumbReady
}

func (t *Thumbs) Forget(id ids.AssetID) {
	if th, ok := t.items[id]; ok {
		if !th.tex.IsZero() {
			t.ctx.Engine.GPU().DestroyTexture(th.tex)
		}
		delete(t.items, id)
	}
}

func (t *Thumbs) Busy() bool {
	return t.inflight > 0
}

func (t *Thumbs) Close() {
	for id := range t.items {
		t.Forget(id)
	}
}
