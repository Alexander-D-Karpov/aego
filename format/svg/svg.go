// Package svg draws a subset of SVG into an image: shapes, paths, fill, stroke, opacity, transforms and groups, without filters, gradients, text or CSS (D363).
package svg

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"strconv"
	"strings"
)

// style is what an element inherits from its groups.
type style struct {
	fill, stroke             paint
	width                    float64
	opacity, fillOp, strokeO float64
	evenOdd                  bool
	cap                      string
	m                        matrix
}

type paint struct {
	c    color.NRGBA
	none bool
}

// Result is the picture and what it could not draw.
type Result struct {
	Image *image.NRGBA
	// Skipped names the elements and attributes outside the subset, once each.
	Skipped []string
}

// Render draws an SVG document into a w×h image; the viewBox is fitted and centred as preserveAspectRatio's default does.
func Render(data []byte, w, h int) (*Result, error) {
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil, fmt.Errorf("svg: size %dx%d is outside 1..4096", w, h)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	r := &Result{Image: image.NewNRGBA(image.Rect(0, 0, w, h))}
	skipped := map[string]bool{}
	skip := func(what string) {
		if !skipped[what] {
			skipped[what] = true
			r.Skipped = append(r.Skipped, what)
		}
	}
	base := style{fill: paint{c: color.NRGBA{A: 255}}, stroke: paint{none: true}, width: 1, opacity: 1, fillOp: 1, strokeO: 1, cap: "butt", m: identity()}
	var stack []style
	seenRoot := false
	depthSkip := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("svg: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depthSkip > 0 {
				depthSkip++
				continue
			}
			parent := base
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			name := t.Name.Local
			if !seenRoot {
				if name != "svg" {
					return nil, errors.New("svg: the document does not start with <svg>")
				}
				seenRoot = true
				parent.m = fit(t.Attr, w, h)
				stack = append(stack, parent)
				continue
			}
			st, err := inherit(parent, t.Attr, skip)
			if err != nil {
				return nil, err
			}
			switch name {
			case "g", "svg":
				stack = append(stack, st)
				continue
			case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
				subs, err := shape(name, t.Attr)
				if err != nil {
					return nil, err
				}
				draw(r.Image, subs, st)
			case "title", "desc", "metadata":
			default:
				skip("<" + name + ">")
			}
			depthSkip = 1
		case xml.EndElement:
			if depthSkip > 0 {
				depthSkip--
				continue
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if !seenRoot {
		return nil, errors.New("svg: no <svg> element")
	}
	return r, nil
}

func attr(as []xml.Attr, name string) (string, bool) {
	for _, a := range as {
		if a.Name.Local == name {
			return strings.TrimSpace(a.Value), true
		}
	}
	return "", false
}

func num(as []xml.Attr, name string) float64 {
	v, _ := attr(as, name)
	f, _ := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
	return f
}

// fit maps the viewBox onto the image, meet and centred.
func fit(as []xml.Attr, w, h int) matrix {
	vb := [4]float64{0, 0, float64(w), float64(h)}
	if v, ok := attr(as, "viewBox"); ok {
		if f := numbers(v); len(f) == 4 && f[2] > 0 && f[3] > 0 {
			copy(vb[:], f)
		}
	} else if aw, ah := num(as, "width"), num(as, "height"); aw > 0 && ah > 0 {
		vb[2], vb[3] = aw, ah
	}
	s := math.Min(float64(w)/vb[2], float64(h)/vb[3])
	dx := (float64(w) - vb[2]*s) / 2
	dy := (float64(h) - vb[3]*s) / 2
	return matrix{s, 0, 0, s, dx - vb[0]*s, dy - vb[1]*s}
}

// inherit applies an element's presentation attributes, style attribute and transform over its parent's.
func inherit(p style, as []xml.Attr, skip func(string)) (style, error) {
	props := map[string]string{}
	for _, a := range as {
		props[a.Name.Local] = strings.TrimSpace(a.Value)
	}
	if css, ok := props["style"]; ok {
		for _, decl := range strings.Split(css, ";") {
			if k, v, ok := strings.Cut(decl, ":"); ok {
				props[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
	}
	st := p
	st.opacity = 1
	for k, v := range props {
		var err error
		switch k {
		case "fill":
			st.fill, err = parsePaint(v)
		case "stroke":
			st.stroke, err = parsePaint(v)
		case "stroke-width":
			st.width, _ = strconv.ParseFloat(strings.TrimSuffix(v, "px"), 64)
		case "opacity":
			st.opacity = unit(v)
		case "fill-opacity":
			st.fillOp = unit(v)
		case "stroke-opacity":
			st.strokeO = unit(v)
		case "fill-rule":
			st.evenOdd = v == "evenodd"
		case "stroke-linecap":
			st.cap = v
		case "transform":
			m, terr := parseTransform(v)
			if terr != nil {
				return st, terr
			}
			st.m = p.m.mul(m)
		case "filter", "mask", "clip-path", "stroke-dasharray", "font-family", "font-size":
			skip(k)
		}
		if err != nil {
			return st, err
		}
	}
	st.opacity *= p.opacity
	return st, nil
}

func unit(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	if err != nil {
		return 1
	}
	if strings.HasSuffix(v, "%") {
		f /= 100
	}
	return math.Max(0, math.Min(1, f))
}

var named = map[string]color.NRGBA{
	"black": {0, 0, 0, 255}, "white": {255, 255, 255, 255}, "red": {255, 0, 0, 255}, "green": {0, 128, 0, 255},
	"blue": {0, 0, 255, 255}, "yellow": {255, 255, 0, 255}, "orange": {255, 165, 0, 255}, "purple": {128, 0, 128, 255},
	"gray": {128, 128, 128, 255}, "grey": {128, 128, 128, 255}, "brown": {165, 42, 42, 255}, "pink": {255, 192, 203, 255},
	"cyan": {0, 255, 255, 255}, "magenta": {255, 0, 255, 255}, "lime": {0, 255, 0, 255}, "navy": {0, 0, 128, 255},
	"gold": {255, 215, 0, 255}, "silver": {192, 192, 192, 255}, "maroon": {128, 0, 0, 255}, "olive": {128, 128, 0, 255},
	"teal": {0, 128, 128, 255}, "currentColor": {0, 0, 0, 255},
}

func parsePaint(v string) (paint, error) {
	if v == "none" || v == "transparent" {
		return paint{none: true}, nil
	}
	c, err := ParseColor(v)
	return paint{c: c}, err
}

// ParseColor reads #rgb, #rrggbb, #rrggbbaa, rgb(r,g,b), rgba(r,g,b,a) and the basic color names.
func ParseColor(v string) (color.NRGBA, error) {
	v = strings.TrimSpace(v)
	if c, ok := named[v]; ok {
		return c, nil
	}
	if strings.HasPrefix(v, "#") {
		h := v[1:]
		if len(h) == 3 || len(h) == 4 {
			var b strings.Builder
			for _, r := range h {
				b.WriteRune(r)
				b.WriteRune(r)
			}
			h = b.String()
		}
		if len(h) == 6 {
			h += "ff"
		}
		if len(h) == 8 {
			if n, err := strconv.ParseUint(h, 16, 32); err == nil {
				return color.NRGBA{uint8(n >> 24), uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
			}
		}
	}
	if inner, ok := strings.CutPrefix(v, "rgba("); ok {
		f := numbers(strings.TrimSuffix(inner, ")"))
		if len(f) == 4 {
			return color.NRGBA{clamp8(f[0]), clamp8(f[1]), clamp8(f[2]), clamp8(f[3] * 255)}, nil
		}
	}
	if inner, ok := strings.CutPrefix(v, "rgb("); ok {
		f := numbers(strings.TrimSuffix(inner, ")"))
		if len(f) == 3 {
			return color.NRGBA{clamp8(f[0]), clamp8(f[1]), clamp8(f[2]), 255}, nil
		}
	}
	return color.NRGBA{}, fmt.Errorf("svg: %q is not a color", v)
}

func clamp8(f float64) uint8 {
	return uint8(math.Max(0, math.Min(255, math.Round(f))))
}

// numbers splits a list of numbers separated by spaces, commas or signs.
func numbers(s string) []float64 {
	var out []float64
	p := scanner{s: s}
	for {
		p.space()
		if p.done() {
			return out
		}
		f, ok := p.number()
		if !ok {
			return out
		}
		out = append(out, f)
	}
}
