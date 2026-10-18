package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

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

type debugFontJSON struct {
	LineHeight float32     `json:"lineHeight"`
	Base       float32     `json:"base"`
	Space      float32     `json:"space"`
	Glyphs     []glyphJSON `json:"glyphs"`
	White      whiteJSON   `json:"white"`
}

type kernJSON struct {
	A int32   `json:"a"`
	B int32   `json:"b"`
	K float32 `json:"k"`
}

type uiFontJSON struct {
	Name       string      `json:"name"`
	Size       float32     `json:"size"`
	Scale      float32     `json:"scale"`
	LineHeight float32     `json:"lineHeight"`
	Ascent     float32     `json:"ascent"`
	Glyphs     []glyphJSON `json:"glyphs"`
	Kerning    []kernJSON  `json:"kerning"`
	White      whiteJSON   `json:"white"`
}

type bakedGlyph struct {
	r    rune
	img  *image.Alpha
	dr   image.Rectangle
	adv  fixed.Int26_6
	x, y int
}

func main() {
	out := flag.String("out", "../../engine/res/fonts", "output directory")
	ttf := flag.String("ttf", "", "OFL font (Inter or Noto Sans) to bake ui atlases from")
	mono := flag.String("mono", "", "OFL monospace font (JetBrains Mono) to bake code atlases from")
	sizes := flag.String("sizes", "12,14,16,20", "font sizes in logical pixels")
	scales := flag.String("scales", "1,2", "atlas scales")
	name := flag.String("name", "ui", "ui atlas file prefix")
	monoName := flag.String("mononame", "ui-mono", "code atlas file prefix")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	if err := bakeDebug(*out); err != nil {
		fail(err)
	}
	if *ttf == "" && *mono == "" {
		fmt.Println("no -ttf or -mono given, only the debug font was baked")
		return
	}
	charset := uiCharset()
	sz, sc := parseInts(*sizes), parseInts(*scales)
	if *ttf != "" {
		fnt := parseFont(*ttf)
		for _, size := range sz {
			for _, scale := range sc {
				if err := bakeUI(fnt, *out, *name, size, scale, charset); err != nil {
					fail(err)
				}
			}
		}
	}
	if *mono != "" {
		fnt := parseFont(*mono)
		for _, size := range sz {
			for _, scale := range sc {
				if err := bakeMono(fnt, *out, *monoName, size, scale, charset); err != nil {
					fail(err)
				}
			}
		}
	}
	fmt.Println("remember to copy the font licenses into", filepath.Join(*out, "LICENSE"))
}

func compose(glyphs []*bakedGlyph, w, h int) (*image.NRGBA, int, int) {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for _, g := range glyphs {
		if g.img == nil {
			continue
		}
		b := g.img.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				if a := g.img.AlphaAt(x, y).A; a != 0 {
					img.SetNRGBA(g.x+x, g.y+y, color.NRGBA{R: 255, G: 255, B: 255, A: a})
				}
			}
		}
	}
	wx, wy := w-3, h-3
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(wx+x, wy+y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return img, wx, wy
}

func bakeMono(fnt *opentype.Font, out, name string, size, scale int, charset []rune) error {
	face, err := opentype.NewFace(fnt, &opentype.FaceOptions{Size: float64(size * scale), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return err
	}
	defer face.Close()
	m := face.Metrics()
	ascent := m.Ascent.Ceil()
	lineH := m.Height.Ceil()
	if lineH <= 0 {
		lineH = ascent + m.Descent.Ceil()
	}
	cell, ok := face.GlyphAdvance('0')
	if !ok {
		return fmt.Errorf("%s: the font has no '0'", name)
	}
	cellW := int(math.Ceil(float64(cell) / 64))
	var buf sfnt.Buffer
	var glyphs []*bakedGlyph
	var missing []rune
	for _, r := range charset {
		ascii := r >= 0x20 && r <= 0x7E
		idx, err := fnt.GlyphIndex(&buf, r)
		if err != nil || (idx == 0 && r != ' ') {
			if ascii {
				missing = append(missing, r)
			}
			continue
		}
		adv, ok := face.GlyphAdvance(r)
		if !ok {
			continue
		}
		if adv != cell {
			if ascii {
				return fmt.Errorf("%s: %q advance %v differs from the cell %v, the font is not monospaced", name, r, adv, cell)
			}
			continue
		}
		g := &bakedGlyph{r: r, adv: adv}
		if dr, mask, mp, _, ok := face.Glyph(fixed.P(0, ascent), r); ok && !dr.Empty() {
			g.img = image.NewAlpha(image.Rect(0, 0, cellW, lineH))
			draw.Draw(g.img, dr, mask, mp, draw.Over)
			g.dr = g.img.Bounds()
		}
		glyphs = append(glyphs, g)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing ASCII glyphs %q", name, string(missing))
	}
	w, h := packGlyphs(glyphs, 1)
	img, wx, wy := compose(glyphs, w, h)
	s := float32(scale)
	meta := uiFontJSON{
		Name:       name + "-" + strconv.Itoa(size),
		Size:       float32(size),
		Scale:      s,
		LineHeight: float32(lineH) / s,
		Ascent:     float32(ascent) / s,
		White:      whiteJSON{X: wx, Y: wy, W: 2, H: 2},
	}
	for _, g := range glyphs {
		gj := glyphJSON{C: int(g.r), Advance: float32(g.adv) / 64 / s}
		if g.img != nil {
			gj.X, gj.Y, gj.W, gj.H = g.x, g.y, cellW, lineH
		}
		meta.Glyphs = append(meta.Glyphs, gj)
	}
	base := filepath.Join(out, name+"-"+strconv.Itoa(size)+"@"+strconv.Itoa(scale))
	if err := writePNG(base+".png", img); err != nil {
		return err
	}
	blob, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".json", append(blob, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%dx%d, cell %dx%d, %d glyphs)\n", base, w, h, cellW, lineH, len(meta.Glyphs))
	return nil
}

func parseFont(p string) *opentype.Font {
	data, err := os.ReadFile(p)
	if err != nil {
		fail(err)
	}
	f, err := opentype.Parse(data)
	if err != nil {
		fail(err)
	}
	return f
}

func parseInts(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.Atoi(part)
		if err != nil || v <= 0 {
			fail(fmt.Errorf("bad number %q", part))
		}
		out = append(out, v)
	}
	return out
}

func uiCharset() []rune {
	var out []rune
	add := func(lo, hi rune) {
		for r := lo; r <= hi; r++ {
			out = append(out, r)
		}
	}
	add(0x20, 0x7E)
	add(0xA0, 0xFF)
	add(0x400, 0x45F)
	add(0x490, 0x491)
	out = append(out,
		0x2013, 0x2014, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2026,
		0x2116, 0x20BD, 0x2190, 0x2191, 0x2192, 0x2193, 0x25B8, 0x25BE,
		0x2713, 0x2715, 0x2318, 0x21E7, 0x2325, 0x2039, 0x203A, 0x25CF, 0x2212,
	)
	return out
}

func bakeUI(fnt *opentype.Font, out, name string, size, scale int, charset []rune) error {
	face, err := opentype.NewFace(fnt, &opentype.FaceOptions{
		Size:    float64(size * scale),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return err
	}
	defer face.Close()
	m := face.Metrics()
	ascent := m.Ascent.Ceil()
	lineH := m.Height.Ceil()
	if lineH <= 0 {
		lineH = ascent + m.Descent.Ceil()
	}

	var buf sfnt.Buffer
	var glyphs []*bakedGlyph
	for _, r := range charset {
		idx, err := fnt.GlyphIndex(&buf, r)
		if err != nil || (idx == 0 && r != ' ') {
			continue
		}
		dr, mask, mp, adv, ok := face.Glyph(fixed.P(0, 0), r)
		if !ok {
			continue
		}
		g := &bakedGlyph{r: r, dr: dr, adv: adv}
		if !dr.Empty() {
			g.img = image.NewAlpha(image.Rect(0, 0, dr.Dx(), dr.Dy()))
			draw.Draw(g.img, g.img.Bounds(), mask, mp, draw.Src)
		}
		glyphs = append(glyphs, g)
	}

	w, h := packGlyphs(glyphs, 1)
	img, wx, wy := compose(glyphs, w, h)

	s := float32(scale)
	meta := uiFontJSON{
		Name:       name + "-" + strconv.Itoa(size),
		Size:       float32(size),
		Scale:      s,
		LineHeight: float32(lineH) / s,
		Ascent:     float32(ascent) / s,
		White:      whiteJSON{X: wx, Y: wy, W: 2, H: 2},
	}
	present := make([]rune, 0, len(glyphs))
	for _, g := range glyphs {
		gj := glyphJSON{C: int(g.r), Advance: float32(g.adv) / 64 / s}
		if g.img != nil {
			gj.X, gj.Y = g.x, g.y
			gj.W, gj.H = g.dr.Dx(), g.dr.Dy()
			gj.OffX = float32(g.dr.Min.X) / s
			gj.OffY = float32(g.dr.Min.Y+ascent) / s
		}
		meta.Glyphs = append(meta.Glyphs, gj)
		present = append(present, g.r)
	}
	ppem := fixed.I(size * scale)
	index := make([]sfnt.GlyphIndex, len(present))
	for i, r := range present {
		index[i], _ = fnt.GlyphIndex(&buf, r)
	}
	for i, a := range present {
		for j, b := range present {
			if k, err := fnt.Kern(&buf, index[i], index[j], ppem, font.HintingFull); err == nil && k != 0 {
				meta.Kerning = append(meta.Kerning, kernJSON{A: int32(a), B: int32(b), K: float32(k) / 64 / s})
			}
		}
	}

	base := filepath.Join(out, name+"-"+strconv.Itoa(size)+"@"+strconv.Itoa(scale))
	if err := writePNG(base+".png", img); err != nil {
		return err
	}
	blob, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(base+".json", append(blob, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%dx%d, %d glyphs, %d kerning pairs)\n", base, w, h, len(meta.Glyphs), len(meta.Kerning))
	return nil
}

func packGlyphs(glyphs []*bakedGlyph, pad int) (int, int) {
	order := make([]*bakedGlyph, 0, len(glyphs))
	for _, g := range glyphs {
		if g.img != nil {
			order = append(order, g)
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].dr.Dy() != order[j].dr.Dy() {
			return order[i].dr.Dy() > order[j].dr.Dy()
		}
		return order[i].r < order[j].r
	})
	for width := 128; ; width *= 2 {
		x, y, shelf := pad, pad, 0
		ok := true
		for _, g := range order {
			gw, gh := g.dr.Dx()+pad, g.dr.Dy()+pad
			if gw+pad > width {
				ok = false
				break
			}
			if x+gw > width {
				x = pad
				y += shelf
				shelf = 0
			}
			g.x, g.y = x, y
			x += gw
			shelf = max(shelf, gh)
		}
		height := nextPow2(y + shelf + 4)
		if ok && (height <= width || width >= 2048) {
			return width, height
		}
	}
}

func bakeDebug(out string) error {
	const cols, pad = 16, 1
	face := basicfont.Face7x13
	cellW := face.Width + pad*2
	cellH := face.Height + pad*2
	first, last := 32, 126
	count := last - first + 1
	rows := (count + cols - 1) / cols
	imgW := nextPow2(cellW * cols)
	imgH := nextPow2(cellH * rows)

	img := image.NewNRGBA(image.Rect(0, 0, imgW, imgH))
	meta := debugFontJSON{
		LineHeight: float32(face.Height),
		Base:       float32(face.Ascent),
		Space:      7,
		Glyphs:     make([]glyphJSON, 0, count),
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.NRGBA{R: 255, G: 255, B: 255, A: 255}), Face: face}
	for i := 0; i < count; i++ {
		c := rune(first + i)
		cx := (i % cols) * cellW
		cy := (i / cols) * cellH
		adv, ok := face.GlyphAdvance(c)
		if !ok {
			continue
		}
		d.Dot = fixed.P(cx+pad, cy+pad+face.Ascent)
		d.DrawString(string(c))
		meta.Glyphs = append(meta.Glyphs, glyphJSON{
			C: int(c), X: cx + pad, Y: cy + pad, W: face.Width, H: face.Height,
			Advance: float32(adv) / 64,
		})
		if c == ' ' {
			meta.Space = float32(adv) / 64
		}
	}
	wx, wy := imgW-4, imgH-4
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.Set(wx+x, wy+y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	meta.White = whiteJSON{X: wx, Y: wy, W: 2, H: 2}
	if err := writePNG(filepath.Join(out, "debug.png"), img); err != nil {
		return err
	}
	blob, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "debug.json"), append(blob, '\n'), 0o644)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func nextPow2(v int) int {
	n := 1
	for n < v {
		n <<= 1
	}
	return n
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genfont:", err)
	os.Exit(1)
}
