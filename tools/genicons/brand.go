package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const (
	brandID     = "app"
	brandRune   = 'æ'
	brandBG     = "#1d2026"
	brandAccent = "#6e9bf5"
	brandBox    = 16.0
	brandInset  = 0.5
	brandRadius = 3.5
	brandGlyphW = 11.0
	brandGlyphH = 8.5
)

var brandSizes = []int{16, 24, 32, 48, 64, 128, 180, 256, 512, 1024}

func fx(v fixed.Int26_6) float64 {
	return float64(v) / 64
}

func fnum(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}

func roundRect(x, y, w, h, r float64, cw bool) string {
	c := r * 0.5523
	m := func(v float64) string {
		if !cw {
			v = 2*x + w - v
		}
		return fnum(v)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M%s %sL%s %s", m(x+r), fnum(y), m(x+w-r), fnum(y))
	fmt.Fprintf(&b, "C%s %s %s %s %s %s", m(x+w-r+c), fnum(y), m(x+w), fnum(y+r-c), m(x+w), fnum(y+r))
	fmt.Fprintf(&b, "L%s %s", m(x+w), fnum(y+h-r))
	fmt.Fprintf(&b, "C%s %s %s %s %s %s", m(x+w), fnum(y+h-r+c), m(x+w-r+c), fnum(y+h), m(x+w-r), fnum(y+h))
	fmt.Fprintf(&b, "L%s %s", m(x+r), fnum(y+h))
	fmt.Fprintf(&b, "C%s %s %s %s %s %s", m(x+r-c), fnum(y+h), m(x), fnum(y+h-r+c), m(x), fnum(y+h-r))
	fmt.Fprintf(&b, "L%s %s", m(x), fnum(y+r))
	fmt.Fprintf(&b, "C%s %s %s %s %s %sZ", m(x), fnum(y+r-c), m(x+r-c), fnum(y), m(x+r), fnum(y))
	return b.String()
}

func writeGlyphSVG(fontGlob, out string) error {
	files, err := filepath.Glob(fontGlob)
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no font matches %s", fontGlob)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		return err
	}
	var buf sfnt.Buffer
	gi, err := f.GlyphIndex(&buf, brandRune)
	if err != nil {
		return err
	}
	if gi == 0 {
		return fmt.Errorf("%s has no %q", files[0], brandRune)
	}
	segs, err := f.LoadGlyph(&buf, gi, fixed.I(1000), nil)
	if err != nil {
		return err
	}
	bb := segs.Bounds()
	x0, y0 := fx(bb.Min.X), fx(bb.Min.Y)
	w, h := fx(bb.Max.X)-x0, fx(bb.Max.Y)-y0
	k := math.Min(brandGlyphW/w, brandGlyphH/h)
	ox, oy := (brandBox-w*k)/2-x0*k, (brandBox-h*k)/2-y0*k
	p := func(q fixed.Point26_6) (float64, float64) {
		return fx(q.X)*k + ox, fx(q.Y)*k + oy
	}
	var d strings.Builder
	var area, sx, sy, lx, ly float64
	open := false
	end := func() {
		if open {
			area += lx*sy - sx*ly
			d.WriteString("Z")
			open = false
		}
	}
	edge := func(x, y float64) {
		area += lx*y - x*ly
		lx, ly = x, y
	}
	for _, s := range segs {
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			end()
			x, y := p(s.Args[0])
			sx, sy, lx, ly, open = x, y, x, y, true
			fmt.Fprintf(&d, "M%s %s", fnum(x), fnum(y))
		case sfnt.SegmentOpLineTo:
			x, y := p(s.Args[0])
			edge(x, y)
			fmt.Fprintf(&d, "L%s %s", fnum(x), fnum(y))
		case sfnt.SegmentOpQuadTo:
			cx, cy := p(s.Args[0])
			x, y := p(s.Args[1])
			edge(x, y)
			fmt.Fprintf(&d, "Q%s %s %s %s", fnum(cx), fnum(cy), fnum(x), fnum(y))
		case sfnt.SegmentOpCubeTo:
			ax, ay := p(s.Args[0])
			bx, by := p(s.Args[1])
			x, y := p(s.Args[2])
			edge(x, y)
			fmt.Fprintf(&d, "C%s %s %s %s %s %s", fnum(ax), fnum(ay), fnum(bx), fnum(by), fnum(x), fnum(y))
		}
	}
	end()
	bg := roundRect(brandInset, brandInset, brandBox-2*brandInset, brandBox-2*brandInset, brandRadius, area < 0)
	svg := fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 16 16\"><path fill=\"%s\" d=\"%s\"/><path fill=\"%s\" d=\"%s\"/></svg>\n",
		brandBG, bg, brandAccent, d.String())
	if err := os.WriteFile(out, []byte(svg), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s from %s\n", out, filepath.Base(files[0]))
	return nil
}

func over(dst *image.NRGBA, mask *image.Alpha, c color.NRGBA) {
	for i, m := range mask.Pix {
		if m == 0 {
			continue
		}
		sa := float64(c.A) / 255 * float64(m) / 255
		o := i * 4
		da := float64(dst.Pix[o+3]) / 255
		oa := sa + da*(1-sa)
		for k, sc := range [3]uint8{c.R, c.G, c.B} {
			dc := float64(dst.Pix[o+k])
			dst.Pix[o+k] = uint8(math.Round((float64(sc)*sa + dc*da*(1-sa)) / oa))
		}
		dst.Pix[o+3] = uint8(math.Round(oa * 255))
	}
}

func renderBrand(ic icon, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, n := range brandSizes {
		img := image.NewNRGBA(image.Rect(0, 0, n, n))
		for _, l := range ic.layers {
			if l.fill.A > 0 {
				over(img, rasterSegs(ic.vb, l.segs, n), l.fill)
			}
		}
		if err := writePNG(filepath.Join(dir, "aego-"+strconv.Itoa(n)+".png"), img); err != nil {
			return err
		}
	}
	return nil
}
