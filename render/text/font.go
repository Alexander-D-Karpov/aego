package text

import (
	"encoding/json"

	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/vfs"
)

type FontGlyph struct {
	Region  texture.Region
	Size    mathx.Vec2
	Offset  mathx.Vec2
	Advance float32
}

type Font struct {
	Name       string
	Size       float32
	Scale      float32
	LineHeight float32
	Ascent     float32
	Texture    gpu.TextureHandle
	White      texture.Region
	ascii      [128]FontGlyph
	asciiOK    [128]bool
	glyphs     map[rune]FontGlyph
	kern       map[uint64]float32
	space      float32
	fallback   rune
}

func NewFont(size, scale, lineHeight, ascent float32) *Font {
	if scale <= 0 {
		scale = 1
	}
	return &Font{
		Size:       size,
		Scale:      scale,
		LineHeight: lineHeight,
		Ascent:     ascent,
		glyphs:     map[rune]FontGlyph{},
		kern:       map[uint64]float32{},
		fallback:   '?',
	}
}

func kernKey(a, b rune) uint64 {
	return uint64(uint32(a))<<32 | uint64(uint32(b))
}

func (f *Font) SetGlyph(r rune, g FontGlyph) {
	if r >= 0 && r < 128 {
		f.ascii[r] = g
		f.asciiOK[r] = true
	} else {
		f.glyphs[r] = g
	}
	if r == ' ' {
		f.space = g.Advance
	}
}

func (f *Font) SetKern(a, b rune, k float32) {
	if k == 0 {
		delete(f.kern, kernKey(a, b))
		return
	}
	f.kern[kernKey(a, b)] = k
}

func (f *Font) SetFallback(r rune) {
	f.fallback = r
}

func (f *Font) Glyph(r rune) (FontGlyph, bool) {
	if r >= 0 && r < 128 {
		return f.ascii[r], f.asciiOK[r]
	}
	g, ok := f.glyphs[r]
	return g, ok
}

func (f *Font) Has(r rune) bool {
	_, ok := f.Glyph(r)
	return ok
}

func (f *Font) spaceWidth() float32 {
	if f.space > 0 {
		return f.space
	}
	return f.LineHeight * 0.3
}

func (f *Font) Advance(r rune) float32 {
	if g, ok := f.Glyph(r); ok {
		return g.Advance
	}
	if r == '\t' {
		return f.spaceWidth() * 4
	}
	if r == ' ' {
		return f.spaceWidth()
	}
	if g, ok := f.Glyph(f.fallback); ok {
		return g.Advance
	}
	return f.spaceWidth()
}

func (f *Font) Kern(a, b rune) float32 {
	if len(f.kern) == 0 {
		return 0
	}
	return f.kern[kernKey(a, b)]
}

func (f *Font) Measure(s string) mathx.Vec2 {
	var w, maxW float32
	lines := float32(1)
	prev := rune(-1)
	for _, r := range s {
		if r == '\n' {
			maxW = max(maxW, w)
			w = 0
			lines++
			prev = -1
			continue
		}
		if prev >= 0 {
			w += f.Kern(prev, r)
		}
		w += f.Advance(r)
		prev = r
	}
	return mathx.Vec2{X: max(maxW, w), Y: lines * f.LineHeight}
}

func (f *Font) WidthRunes(rs []rune) float32 {
	return f.XRunes(rs, len(rs))
}

func (f *Font) XRunes(rs []rune, n int) float32 {
	if n > len(rs) {
		n = len(rs)
	}
	var x float32
	prev := rune(-1)
	for i := 0; i < n; i++ {
		r := rs[i]
		if prev >= 0 {
			x += f.Kern(prev, r)
		}
		x += f.Advance(r)
		prev = r
	}
	return x
}

func (f *Font) IndexAtRunes(rs []rune, x float32) int {
	var cur float32
	prev := rune(-1)
	for i, r := range rs {
		if prev >= 0 {
			cur += f.Kern(prev, r)
		}
		adv := f.Advance(r)
		if x < cur+adv/2 {
			return i
		}
		cur += adv
		prev = r
	}
	return len(rs)
}

func (f *Font) Layout(s string, pos mathx.Vec2, scale float32, c mathx.Color, layer, order int16, out []sprite.Draw) []sprite.Draw {
	if scale <= 0 {
		scale = 1
	}
	x, y := pos.X, pos.Y
	prev := rune(-1)
	for _, r := range s {
		if r == '\n' {
			x = pos.X
			y += f.LineHeight * scale
			prev = -1
			continue
		}
		if prev >= 0 {
			x += f.Kern(prev, r) * scale
		}
		prev = r
		g, ok := f.Glyph(r)
		if !ok && r != ' ' && r != '\t' {
			g, ok = f.Glyph(f.fallback)
		}
		if !ok {
			x += f.Advance(r) * scale
			continue
		}
		if !g.Region.IsZero() && g.Size.X > 0 && g.Size.Y > 0 {
			out = append(out, sprite.Draw{
				Region:   g.Region,
				Position: mathx.Vec2{X: x + g.Offset.X*scale, Y: y + g.Offset.Y*scale},
				Size:     g.Size.Scale(scale),
				Tint:     c,
				Layer:    layer,
				Order:    order,
			})
		}
		x += g.Advance * scale
	}
	return out
}

type fontGlyphJSON struct {
	C       int32   `json:"c"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	W       int     `json:"w"`
	H       int     `json:"h"`
	Advance float32 `json:"advance"`
	OffX    float32 `json:"offX"`
	OffY    float32 `json:"offY"`
}

type kernJSON struct {
	A int32   `json:"a"`
	B int32   `json:"b"`
	K float32 `json:"k"`
}

type fontFileJSON struct {
	Name       string          `json:"name"`
	Size       float32         `json:"size"`
	Scale      float32         `json:"scale"`
	LineHeight float32         `json:"lineHeight"`
	Ascent     float32         `json:"ascent"`
	Glyphs     []fontGlyphJSON `json:"glyphs"`
	Kerning    []kernJSON      `json:"kerning"`
	White      whiteJSON       `json:"white"`
}

func LoadFont(files vfs.FS, up *texture.Uploader, pngPath, jsonPath string) (*Font, error) {
	meta, err := files.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var fj fontFileJSON
	if err := json.Unmarshal(meta, &fj); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "font metadata is not valid json").With(log.F("path", jsonPath))
	}
	pngFile, err := files.Open(pngPath)
	if err != nil {
		return nil, err
	}
	defer pngFile.Close()
	data, err := texture.DecodePNG(pngFile, true)
	if err != nil {
		return nil, err
	}
	tex, err := up.Upload(data, texture.UploadOptions{Label: "font." + fj.Name})
	if err != nil {
		return nil, err
	}
	whole := texture.Whole(tex, data.Width, data.Height)
	f := NewFont(fj.Size, fj.Scale, fj.LineHeight, fj.Ascent)
	f.Name = fj.Name
	f.Texture = tex
	for _, g := range fj.Glyphs {
		fg := FontGlyph{
			Offset:  mathx.Vec2{X: g.OffX, Y: g.OffY},
			Advance: g.Advance,
		}
		if g.W > 0 && g.H > 0 {
			fg.Region = whole.Sub(g.X, g.Y, g.W, g.H)
			fg.Size = mathx.Vec2{X: float32(g.W) / f.Scale, Y: float32(g.H) / f.Scale}
		}
		f.SetGlyph(rune(g.C), fg)
	}
	for _, k := range fj.Kerning {
		f.SetKern(rune(k.A), rune(k.B), k.K)
	}
	if fj.White.W > 0 && fj.White.H > 0 {
		f.White = whole.Sub(fj.White.X, fj.White.Y, fj.White.W, fj.White.H)
	}
	if f.LineHeight <= 0 {
		f.LineHeight = f.Size * 1.25
	}
	return f, nil
}
