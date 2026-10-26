package svg

import (
	"image"
	"image/color"
	"math"
	"sort"
)

// sub is the number of sample rows per pixel row; columns are covered exactly.
const sub = 8

type edge struct {
	x0, y0, x1, y1 float64
	dir            int
}

// draw fills and strokes subpaths given in user space.
func draw(img *image.NRGBA, subs []subpath, st style) {
	var dev []subpath
	for _, s := range subs {
		d := subpath{closed: s.closed, pts: make([]pt, len(s.pts))}
		for i, p := range s.pts {
			d.pts[i] = st.m.apply(p)
		}
		dev = append(dev, d)
	}
	if !st.fill.none {
		var polys [][]pt
		for _, s := range dev {
			if len(s.pts) > 2 {
				polys = append(polys, s.pts)
			}
		}
		fill(img, polys, st.evenOdd, st.fill.c, st.opacity*st.fillOp)
	}
	if !st.stroke.none && st.width > 0 {
		w := st.width * st.m.scale()
		fill(img, outline(dev, w, st.cap), false, st.stroke.c, st.opacity*st.strokeO)
	}
}

// outline turns strokes into polygons that all wind the same way, so a nonzero fill unites them.
func outline(subs []subpath, w float64, linecap string) [][]pt {
	h := w / 2
	var out [][]pt
	add := func(p []pt) {
		if area(p) < 0 {
			for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
				p[i], p[j] = p[j], p[i]
			}
		}
		out = append(out, p)
	}
	disc := func(c pt) {
		if h < 0.3 {
			return
		}
		add(ellipse(c.x, c.y, h, h).pts)
	}
	for _, s := range subs {
		pts := s.pts
		if s.closed && len(pts) > 1 {
			pts = append(append([]pt(nil), pts...), pts[0])
		}
		for i := 0; i+1 < len(pts); i++ {
			a, b := pts[i], pts[i+1]
			dx, dy := b.x-a.x, b.y-a.y
			l := math.Hypot(dx, dy)
			if l == 0 {
				continue
			}
			nx, ny := -dy/l*h, dx/l*h
			ex, ey := 0.0, 0.0
			first, last := i == 0 && !s.closed, i+2 == len(pts) && !s.closed
			if linecap == "square" {
				ex, ey = dx/l*h, dy/l*h
			}
			a0, b0 := a, b
			if first {
				a0 = pt{a.x - ex, a.y - ey}
			}
			if last {
				b0 = pt{b.x + ex, b.y + ey}
			}
			add([]pt{{a0.x + nx, a0.y + ny}, {b0.x + nx, b0.y + ny}, {b0.x - nx, b0.y - ny}, {a0.x - nx, a0.y - ny}})
			if !last || s.closed {
				disc(b)
			}
			if (first || last) && linecap == "round" {
				if first {
					disc(a)
				}
				if last {
					disc(b)
				}
			}
		}
	}
	return out
}

func area(p []pt) float64 {
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].x*p[j].y - p[j].x*p[i].y
	}
	return a / 2
}

// fill scan-converts polygons with exact horizontal coverage and sub rows per pixel, and blends the color over img.
func fill(img *image.NRGBA, polys [][]pt, evenOdd bool, c color.NRGBA, opacity float64) {
	b := img.Bounds()
	var edges []edge
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range polys {
		for i := range p {
			a, q := p[i], p[(i+1)%len(p)]
			if a.y == q.y {
				continue
			}
			e := edge{a.x, a.y, q.x, q.y, 1}
			if a.y > q.y {
				e = edge{q.x, q.y, a.x, a.y, -1}
			}
			edges = append(edges, e)
			minY, maxY = math.Min(minY, e.y0), math.Max(maxY, e.y1)
		}
	}
	if len(edges) == 0 {
		return
	}
	w := b.Dx()
	cov := make([]float64, w+1)
	y0 := max(b.Min.Y, int(math.Floor(minY)))
	y1 := min(b.Max.Y, int(math.Ceil(maxY)))
	type cross struct {
		x   float64
		dir int
	}
	var xs []cross
	for y := y0; y < y1; y++ {
		clear(cov)
		touched := false
		for s := 0; s < sub; s++ {
			sy := float64(y) + (float64(s)+0.5)/sub
			xs = xs[:0]
			for _, e := range edges {
				if sy >= e.y0 && sy < e.y1 {
					xs = append(xs, cross{e.x0 + (sy-e.y0)*(e.x1-e.x0)/(e.y1-e.y0), e.dir})
				}
			}
			if len(xs) < 2 {
				continue
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
			wind := 0
			for i := 0; i+1 < len(xs); i++ {
				if evenOdd {
					wind ^= 1
				} else {
					wind += xs[i].dir
				}
				if wind != 0 {
					span(cov, xs[i].x-float64(b.Min.X), xs[i+1].x-float64(b.Min.X), 1.0/sub)
					touched = true
				}
			}
		}
		if !touched {
			continue
		}
		row := img.Pix[(y-b.Min.Y)*img.Stride:]
		for x := 0; x < w; x++ {
			if a := math.Min(1, cov[x]) * opacity * float64(c.A) / 255; a > 0 {
				over(row[x*4:x*4+4], c, a)
			}
		}
	}
}

// span adds the part of each pixel that [a, b) covers.
func span(cov []float64, a, b, weight float64) {
	a, b = math.Max(a, 0), math.Min(b, float64(len(cov)-1))
	if b <= a {
		return
	}
	ia, ib := int(a), int(b)
	if ia == ib {
		cov[ia] += (b - a) * weight
		return
	}
	cov[ia] += (float64(ia+1) - a) * weight
	for x := ia + 1; x < ib; x++ {
		cov[x] += weight
	}
	cov[ib] += (b - float64(ib)) * weight
}

// over blends c with alpha a over a non-premultiplied pixel.
func over(px []uint8, c color.NRGBA, a float64) {
	da := float64(px[3]) / 255
	oa := a + da*(1-a)
	if oa <= 0 {
		return
	}
	for i, sc := range [3]uint8{c.R, c.G, c.B} {
		v := (float64(sc)*a + float64(px[i])*da*(1-a)) / oa
		px[i] = uint8(math.Round(v))
	}
	px[3] = uint8(math.Round(oa * 255))
}
