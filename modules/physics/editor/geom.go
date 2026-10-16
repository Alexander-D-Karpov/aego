package editor

import (
	"math"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/modules/physics/components"
	"aego/render/debug"
	"aego/scene"
)

type hkind uint8

const (
	hMove hkind = iota
	hInsert
)

type handle struct {
	at    mathx.Vec2
	kind  hkind
	index int
	nx    float32
	ny    float32
}

var boxDirs = [8][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}

func handles(c *components.Collider, out []handle) []handle {
	o := c.Common.Offset
	switch c.Kind {
	case components.KindBox:
		hx, hy := c.Size.X*0.5, c.Size.Y*0.5
		for i, d := range boxDirs {
			out = append(out, handle{at: o.Add(mathx.Vec2{X: hx * d[0], Y: hy * d[1]}), index: i, nx: d[0], ny: d[1]})
		}
	case components.KindCircle:
		out = append(out, handle{at: o.Add(mathx.Vec2{X: c.Radius}), nx: 1})
	case components.KindCapsule:
		half := c.Length * 0.5
		out = append(out,
			handle{at: o.Add(mathx.Vec2{X: half}), index: 0, nx: 1},
			handle{at: o.Sub(mathx.Vec2{X: half}), index: 1, nx: -1},
			handle{at: o.Add(mathx.Vec2{Y: -c.Radius}), index: 2, ny: -1},
		)
	case components.KindPolygon, components.KindEdge:
		n := len(c.Points)
		for i, pt := range c.Points {
			out = append(out, handle{at: pt.Add(o), index: i})
		}
		segs := n - 1
		if c.Kind == components.KindPolygon || c.Loop {
			segs = n
		}
		if c.Kind == components.KindPolygon && n >= components.MaxPolygonPoints {
			segs = 0
		}
		for i := 0; i < segs && n > 1; i++ {
			mid := c.Points[i].Lerp(c.Points[(i+1)%n], 0.5)
			out = append(out, handle{at: mid.Add(o), kind: hInsert, index: i})
		}
	}
	return out
}

func arc(pts []mathx.Vec2, center mathx.Vec2, r, from, to float32, segs int) []mathx.Vec2 {
	for i := 0; i <= segs; i++ {
		a := from + (to-from)*float32(i)/float32(segs)
		pts = append(pts, center.Add(mathx.Vec2{X: r * mathx.Cos(a), Y: r * mathx.Sin(a)}))
	}
	return pts
}

func outline(c *components.Collider, pts []mathx.Vec2) ([]mathx.Vec2, bool) {
	pts = pts[:0]
	o := c.Common.Offset
	switch c.Kind {
	case components.KindBox:
		hx, hy := c.Size.X*0.5, c.Size.Y*0.5
		for _, d := range boxDirs[:4] {
			pts = append(pts, o.Add(mathx.Vec2{X: hx * d[0], Y: hy * d[1]}))
		}
		return pts, true
	case components.KindCircle:
		return arc(pts, o, c.Radius, 0, mathx.TwoPi, 32), true
	case components.KindCapsule:
		half := max(c.Length*0.5-c.Radius, 0)
		pts = arc(pts, o.Add(mathx.Vec2{X: half}), c.Radius, -mathx.Pi/2, mathx.Pi/2, 12)
		return arc(pts, o.Sub(mathx.Vec2{X: half}), c.Radius, mathx.Pi/2, 3*mathx.Pi/2, 12), true
	case components.KindPolygon, components.KindEdge:
		for _, pt := range c.Points {
			pts = append(pts, pt.Add(o))
		}
		return pts, c.Kind == components.KindPolygon || c.Loop
	}
	return pts, false
}

func drawPoly(d *debug.Draw, m mathx.Mat3, pts []mathx.Vec2, closed bool, col mathx.Color) {
	for i := 0; i+1 < len(pts); i++ {
		d.Line(m.Apply(pts[i]), m.Apply(pts[i+1]), col)
	}
	if closed && len(pts) > 2 {
		d.Line(m.Apply(pts[len(pts)-1]), m.Apply(pts[0]), col)
	}
}

func convex(pts []mathx.Vec2) bool {
	n := len(pts)
	if n < 3 {
		return false
	}
	sign := 0
	for i := 0; i < n; i++ {
		a, b, c := pts[i], pts[(i+1)%n], pts[(i+2)%n]
		cr := b.Sub(a).Cross(c.Sub(b))
		if mathx.Abs(cr) < 1e-6 {
			return false
		}
		s := 1
		if cr < 0 {
			s = -1
		}
		if sign == 0 {
			sign = s
		} else if s != sign {
			return false
		}
	}
	return true
}

func bounds(k components.Kind) shape.Func {
	return func(s *scene.Scene, _ asset.Resolver, h scene.NodeHandle) (shape.Shape, bool) {
		var c components.Collider
		if !components.Read(s, h, k, &c) {
			return shape.Shape{}, false
		}
		pts, _ := outline(&c, nil)
		if len(pts) == 0 {
			return shape.Shape{}, false
		}
		lo, hi := pts[0], pts[0]
		for _, pt := range pts[1:] {
			lo = mathx.Vec2{X: min(lo.X, pt.X), Y: min(lo.Y, pt.Y)}
			hi = mathx.Vec2{X: max(hi.X, pt.X), Y: max(hi.Y, pt.Y)}
		}
		pad := float32(0.05)
		return shape.Shape{
			Kind:  shape.Rect,
			Local: mathx.Rect{X: lo.X - pad, Y: lo.Y - pad, W: hi.X - lo.X + 2*pad, H: hi.Y - lo.Y + 2*pad},
			Order: math.MinInt32,
		}, true
	}
}
