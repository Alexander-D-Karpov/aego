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

type Glyph struct {
	Region  texture.Region
	Advance float32
	OffX    float32
	OffY    float32
	Valid   bool
}

type BitmapFont struct {
	Glyphs     [256]Glyph
	LineHeight float32
	Base       float32
	Space      float32
	Texture    gpu.TextureHandle
	White      texture.Region
}

type glyphJSON struct {
	C       int     `json:"c"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	W       int     `json:"w"`
	H       int     `json:"h"`
	Advance float32 `json:"advance"`
	OffX    float32 `json:"offX"`
	OffY    float32 `json:"offY"`
}

type whiteJSON struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

type fontJSON struct {
	LineHeight float32     `json:"lineHeight"`
	Base       float32     `json:"base"`
	Space      float32     `json:"space"`
	Glyphs     []glyphJSON `json:"glyphs"`
	White      whiteJSON   `json:"white"`
}

func LoadBitmap(files vfs.FS, up *texture.Uploader, pngPath, jsonPath string) (*BitmapFont, error) {
	pngFile, err := files.Open(pngPath)
	if err != nil {
		return nil, err
	}
	defer pngFile.Close()
	data, err := texture.DecodePNG(pngFile, true)
	if err != nil {
		return nil, err
	}
	meta, err := files.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}
	var fj fontJSON
	if err := json.Unmarshal(meta, &fj); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "font metadata is not valid json").With(log.F("path", jsonPath))
	}
	tex, err := up.Upload(data, texture.UploadOptions{Label: "font.debug"})
	if err != nil {
		return nil, err
	}
	whole := texture.Whole(tex, data.Width, data.Height)
	f := &BitmapFont{
		LineHeight: fj.LineHeight,
		Base:       fj.Base,
		Space:      fj.Space,
		Texture:    tex,
	}
	for _, g := range fj.Glyphs {
		if g.C < 0 || g.C > 255 {
			continue
		}
		f.Glyphs[g.C] = Glyph{
			Region:  whole.Sub(g.X, g.Y, g.W, g.H),
			Advance: g.Advance,
			OffX:    g.OffX,
			OffY:    g.OffY,
			Valid:   true,
		}
	}
	if fj.White.W > 0 && fj.White.H > 0 {
		f.White = whole.Sub(fj.White.X, fj.White.Y, fj.White.W, fj.White.H)
	}
	if f.LineHeight <= 0 {
		f.LineHeight = 13
	}
	if f.Space <= 0 {
		f.Space = f.Glyphs[' '].Advance
	}
	return f, nil
}

func (f *BitmapFont) Font() *Font {
	out := NewFont(f.LineHeight, 1, f.LineHeight, f.Base)
	out.Name = "debug"
	out.Texture = f.Texture
	out.White = f.White
	for c := 0; c < len(f.Glyphs); c++ {
		g := f.Glyphs[c]
		if !g.Valid {
			continue
		}
		out.SetGlyph(rune(c), FontGlyph{
			Region:  g.Region,
			Size:    g.Region.Size(),
			Offset:  mathx.Vec2{X: g.OffX, Y: g.OffY},
			Advance: g.Advance,
		})
	}
	if out.space == 0 {
		out.space = f.Space
	}
	return out
}

func (f *BitmapFont) Measure(s string) mathx.Vec2 {
	var w, maxW float32
	lines := float32(1)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' {
			maxW = max(maxW, w)
			w = 0
			lines++
			continue
		}
		w += f.advance(c)
	}
	return mathx.Vec2{X: max(maxW, w), Y: lines * f.LineHeight}
}

func (f *BitmapFont) advance(c byte) float32 {
	g := f.Glyphs[c]
	if !g.Valid {
		return f.Space
	}
	return g.Advance
}

func (f *BitmapFont) Layout(s string, pos mathx.Vec2, scale float32, color mathx.Color, layer, order int16, out []sprite.Draw) []sprite.Draw {
	if scale <= 0 {
		scale = 1
	}
	x, y := pos.X, pos.Y
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' {
			x = pos.X
			y += f.LineHeight * scale
			continue
		}
		g := f.Glyphs[c]
		if !g.Valid {
			x += f.Space * scale
			continue
		}
		if c != ' ' {
			out = append(out, sprite.Draw{
				Region:   g.Region,
				Position: mathx.Vec2{X: x + g.OffX*scale, Y: y + g.OffY*scale},
				Size:     g.Region.Size().Scale(scale),
				Tint:     color,
				Layer:    layer,
				Order:    order,
			})
		}
		x += g.Advance * scale
	}
	return out
}
