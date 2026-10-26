// Package pixelart turns pixel art written as text, a character grid and a palette, into an image or an atlas of frames (D363).
package pixelart

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"unicode"

	"aego/format/svg"
)

// MaxSide bounds a frame, so a typo cannot ask for a huge texture.
const MaxSide = 256

// Palette maps a grid character to its color; transparent is the zero color.
type Palette map[rune]color.NRGBA

// ParsePalette reads {"r": "#d0312d", ".": "transparent"} or lines and pairs such as r=#d0312d s=#f4c7a1.
func ParsePalette(s string) (Palette, error) {
	pal := Palette{}
	s = strings.TrimSpace(s)
	if s == "" {
		return pal, nil
	}
	pairs := map[string]string{}
	if strings.HasPrefix(s, "{") {
		if err := json.Unmarshal([]byte(s), &pairs); err != nil {
			return nil, fmt.Errorf("palette: %w", err)
		}
	} else {
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == ';' }) {
			k, v, ok := strings.Cut(f, "=")
			if !ok {
				k, v, ok = strings.Cut(f, ":")
			}
			if !ok {
				return nil, fmt.Errorf("palette: %q is not char=color", f)
			}
			pairs[k] = v
		}
	}
	for k, v := range pairs {
		r := []rune(k)
		if len(r) != 1 || unicode.IsSpace(r[0]) {
			return nil, fmt.Errorf("palette: key %q must be one visible character", k)
		}
		v = strings.TrimSpace(v)
		if v == "transparent" || v == "none" || v == "" {
			pal[r[0]] = color.NRGBA{}
			continue
		}
		c, err := svg.ParseColor(v)
		if err != nil {
			return nil, fmt.Errorf("palette: %s: %w", k, err)
		}
		pal[r[0]] = c
	}
	return pal, nil
}

// Keys gives the palette's characters in order, for messages.
func (p Palette) Keys() string {
	var ks []rune
	for k := range p {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return string(ks)
}

// Sheet is the drawn art: one frame, or frames of one size laid out in an atlas.
type Sheet struct {
	Image *image.NRGBA
	// Names are the frames in order; a single unnamed frame has one empty name.
	Names []string
	// Cells are the frames' rectangles in Image.
	Cells []image.Rectangle
}

type frame struct {
	name string
	rows [][]rune
	line int
}

// Parse reads frames introduced by a line "name:" (or one frame without one); '.' and ' ' are transparent unless the palette names them; scale repeats each pixel.
func Parse(text string, pal Palette, scale int) (*Sheet, error) {
	if scale < 1 || scale > 16 {
		return nil, fmt.Errorf("scale %d is outside 1..16", scale)
	}
	var frames []*frame
	cur := &frame{}
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if name, ok := strings.CutSuffix(t, ":"); ok && name != "" && isName(name) {
			if len(cur.rows) > 0 || cur.name != "" {
				frames = append(frames, cur)
			}
			cur = &frame{name: name, line: i + 1}
			continue
		}
		row := []rune(strings.Map(func(r rune) rune {
			if r == '\t' {
				return -1
			}
			return r
		}, t))
		if cur.line == 0 {
			cur.line = i + 1
		}
		cur.rows = append(cur.rows, row)
	}
	if len(cur.rows) > 0 || cur.name != "" {
		frames = append(frames, cur)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("the grid is empty")
	}
	w, h := 0, len(frames[0].rows)
	for _, r := range frames[0].rows {
		w = max(w, len(r))
	}
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("frame %q has no pixels", frames[0].name)
	}
	if w > MaxSide || h > MaxSide {
		return nil, fmt.Errorf("a frame is %dx%d; frames are at most %d pixels a side", w, h, MaxSide)
	}
	seen := map[string]bool{}
	for _, f := range frames {
		if len(frames) > 1 && f.name == "" {
			return nil, fmt.Errorf("line %d: with several frames each starts with a line such as \"idle:\"", f.line)
		}
		if seen[f.name] {
			return nil, fmt.Errorf("frame %q appears twice", f.name)
		}
		seen[f.name] = true
		if len(f.rows) != h {
			return nil, fmt.Errorf("frame %q has %d rows, the first frame %d; frames are one size", f.name, len(f.rows), h)
		}
		for _, r := range f.rows {
			if len(r) != w && len(f.rows) > 0 {
				return nil, fmt.Errorf("frame %q has rows of %d and %d pixels; pad short rows with '.'", f.name, len(r), w)
			}
		}
	}
	cols := min(len(frames), 8)
	rows := (len(frames) + cols - 1) / cols
	fw, fh := w*scale, h*scale
	s := &Sheet{Image: image.NewNRGBA(image.Rect(0, 0, cols*fw, rows*fh))}
	for n, f := range frames {
		ox, oy := n%cols*fw, n/cols*fh
		s.Names = append(s.Names, f.name)
		s.Cells = append(s.Cells, image.Rect(ox, oy, ox+fw, oy+fh))
		for y, r := range f.rows {
			for x, ch := range r {
				c, ok := pal[ch]
				if !ok {
					if ch != '.' && ch != ' ' {
						return nil, fmt.Errorf("frame %q row %d column %d: %q is not in the palette (%s)", f.name, y+1, x+1, ch, pal.Keys())
					}
					continue
				}
				if c.A == 0 {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						s.Image.SetNRGBA(ox+x*scale+dx, oy+y*scale+dy, c)
					}
				}
			}
		}
	}
	return s, nil
}

func isName(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// Upscale repeats each pixel so a small picture can be looked at; nearest, no blur.
func Upscale(img image.Image, factor int) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()*factor, b.Dy()*factor))
	for y := 0; y < out.Bounds().Dy(); y++ {
		for x := 0; x < out.Bounds().Dx(); x++ {
			out.Set(x, y, img.At(b.Min.X+x/factor, b.Min.Y+y/factor))
		}
	}
	return out
}

// OffPalette counts the opaque pixels and those whose color is not in the palette; pixels under half alpha are ignored.
func OffPalette(img image.Image, pal []color.NRGBA) (off, opaque int) {
	in := map[color.NRGBA]bool{}
	for _, c := range pal {
		c.A = 255
		in[c] = true
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < 128 {
				continue
			}
			opaque++
			c.A = 255
			if !in[c] {
				off++
			}
		}
	}
	return off, opaque
}
