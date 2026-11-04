package preview

import (
	"bytes"
	"encoding/json"

	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strings"
)

type glyph struct {
	C                   rune
	X, Y, W, H, Advance int
}

func loadFont(files interface{ ReadFile(string) ([]byte, error) }) (image.Image, map[rune]glyph) {
	if files == nil {
		return nil, nil
	}
	raw, err := files.ReadFile("engine://fonts/debug.json")
	if err != nil {
		return nil, nil
	}
	var desc struct{ Glyphs []glyph }
	if json.Unmarshal(raw, &desc) != nil {
		return nil, nil
	}
	raw, err = files.ReadFile("engine://fonts/debug.png")
	if err != nil {
		return nil, nil
	}
	im, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, nil
	}
	chars := make(map[rune]glyph, len(desc.Glyphs))
	for _, g := range desc.Glyphs {
		chars[g.C] = g
	}
	return im, chars
}

var ink = color.NRGBA{222, 230, 238, 255}
var accent = color.NRGBA{99, 172, 220, 255}
var paper = color.NRGBA{34, 38, 46, 255}

func (s *Service) label(im *image.NRGBA, x, y int, text string, c color.NRGBA) {
	s.fontOnce.Do(func() {
		if s.d.Engine != nil {
			s.font, s.chars = loadFont(s.d.Engine.VFS())
		}
	})
	font, chars := s.font, s.chars
	if font == nil {
		return
	}
	for _, ch := range text {
		g, ok := chars[ch]
		if !ok {
			g = chars['?']
		}
		if x+g.W > im.Rect.Max.X-1 {
			break
		}
		draw.DrawMask(im, image.Rect(x, y, x+g.W, y+g.H), image.NewUniform(c), image.Point{}, font, image.Pt(g.X, g.Y), draw.Over)
		x += g.Advance
	}
}
func canvas() *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, Size, Size))
	draw.Draw(im, im.Rect, image.NewUniform(paper), image.Point{}, draw.Src)
	return im
}
func raster(im *image.NRGBA) pixels { return pixels{buf: im.Pix, w: im.Rect.Dx(), h: im.Rect.Dy()} }
func (s *Service) card(header string, lines []string) pixels {
	im := canvas()
	s.label(im, 4, 4, strings.ToUpper(header), accent)
	y := 21
	for _, line := range lines {
		line = strings.TrimSpace(line)
		s.label(im, 4, y, line, ink)
		y += 14
		if y+13 > Size {
			break
		}
	}
	return raster(im)
}

func fitPixels(im *image.NRGBA, area image.Rectangle, p pixels) {
	if p.w < 1 || p.h < 1 {
		return
	}
	scale := math.Min(float64(area.Dx())/float64(p.w), float64(area.Dy())/float64(p.h))
	w, h := max(1, int(float64(p.w)*scale)), max(1, int(float64(p.h)*scale))
	left := area.Min.X + (area.Dx()-w)/2
	top := area.Min.Y + (area.Dy()-h)/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*p.h/h*p.w + x*p.w/w) * 4
			j := im.PixOffset(left+x, top+y)
			copy(im.Pix[j:j+4], p.buf[i:i+4])
		}
	}
}
