package world

import "aego/core/mathx"

type Drawer interface {
	Line(a, b Vec2, c mathx.Color)
	Circle(center Vec2, radius float32, segments int, c mathx.Color)
}

type DebugFlags uint32

const (
	DrawShapes DebugFlags = 1 << iota
	DrawAABBs
	DrawContacts
	DrawVelocities
	DrawIslands
	DrawJoints
	DrawCenters
)

const DrawDefault = DrawShapes | DrawJoints

const DrawAll = DrawShapes | DrawAABBs | DrawContacts | DrawVelocities | DrawIslands | DrawJoints | DrawCenters

func rgba(r, g, b, a float32) mathx.Color {
	return mathx.Color{R: r, G: g, B: b, A: a}
}

func islandColor(i int32) mathx.Color {
	palette := [...]mathx.Color{
		rgba(0.95, 0.45, 0.35, 1), rgba(0.35, 0.75, 0.95, 1), rgba(0.55, 0.9, 0.4, 1),
		rgba(0.95, 0.8, 0.3, 1), rgba(0.8, 0.45, 0.95, 1), rgba(0.3, 0.9, 0.8, 1),
		rgba(0.95, 0.55, 0.75, 1), rgba(0.65, 0.65, 0.95, 1),
	}
	if i < 0 {
		i = -i
	}
	return palette[int(i)%len(palette)]
}

func (w *World) bodyColor(b *body, flags DebugFlags) mathx.Color {
	switch {
	case b.typ == Static:
		return rgba(0.5, 0.85, 0.5, 1)
	case b.typ == Kinematic:
		return rgba(0.5, 0.6, 0.95, 1)
	case !b.awake:
		return rgba(0.55, 0.55, 0.55, 1)
	case flags&DrawIslands != 0:
		return islandColor(b.island)
	}
	return rgba(0.95, 0.7, 0.55, 1)
}

func drawPoly(d Drawer, pts []Vec2, closed bool, c mathx.Color) {
	for i := 0; i+1 < len(pts); i++ {
		d.Line(pts[i], pts[i+1], c)
	}
	if closed && len(pts) > 2 {
		d.Line(pts[len(pts)-1], pts[0], c)
	}
}

func drawBox(d Drawer, a AABB, c mathx.Color) {
	p0, p1 := a.Min, Vec2{X: a.Max.X, Y: a.Min.Y}
	p2, p3 := a.Max, Vec2{X: a.Min.X, Y: a.Max.Y}
	d.Line(p0, p1, c)
	d.Line(p1, p2, c)
	d.Line(p2, p3, c)
	d.Line(p3, p0, c)
}

func drawCross(d Drawer, p Vec2, size float32, c mathx.Color) {
	h := size / 2
	d.Line(Vec2{X: p.X - h, Y: p.Y}, Vec2{X: p.X + h, Y: p.Y}, c)
	d.Line(Vec2{X: p.X, Y: p.Y - h}, Vec2{X: p.X, Y: p.Y + h}, c)
}

func (w *World) drawShape(d Drawer, s *shape, x xform, c mathx.Color) {
	if s.disabled {
		return
	}
	g := &s.g
	switch g.kind {
	case KindCircle:
		center := x.apply(g.c.v[0])
		d.Circle(center, g.c.radius, 20, c)
		d.Line(center, center.Add(x.q.apply(Vec2{X: g.c.radius})), c)
	case KindCapsule:
		v0, v1 := x.apply(g.c.v[0]), x.apply(g.c.v[1])
		e, _ := normalizeLen(v1.Sub(v0))
		n := rightPerp(e).Scale(g.c.radius)
		d.Line(v0.Add(n), v1.Add(n), c)
		d.Line(v0.Sub(n), v1.Sub(n), c)
		d.Circle(v0, g.c.radius, 16, c)
		d.Circle(v1, g.c.radius, 16, c)
	case KindBox, KindPolygon:
		var pts [maxPolyVerts]Vec2
		for i := 0; i < g.c.count; i++ {
			pts[i] = x.apply(g.c.v[i])
		}
		drawPoly(d, pts[:g.c.count], true, c)
	case KindChain:
		ch := g.chain
		for i := 0; i < ch.segments(); i++ {
			a, b := x.apply(ch.point(i)), x.apply(ch.point(i+1))
			d.Line(a, b, c)
			if s.oneSided {
				e, _ := normalizeLen(b.Sub(a))
				mid := a.Lerp(b, 0.5)
				d.Line(mid, mid.Add(rightPerp(e).Scale(0.2)), c)
			}
		}
	}
}

func (w *World) DebugDraw(d Drawer, flags DebugFlags) {
	trigger := rgba(1, 0.9, 0.3, 0.6)
	for i := range w.bodies.items {
		if !w.bodies.alive[i] {
			continue
		}
		b := &w.bodies.items[i]
		if b.dead {
			continue
		}
		col := w.bodyColor(b, flags)
		if flags&DrawShapes != 0 {
			for _, sh := range b.shapes {
				s := &w.shapes.items[sh.Index]
				c := col
				if s.trigger {
					c = trigger
				}
				w.drawShape(d, s, b.xf, c)
			}
		}
		if flags&DrawCenters != 0 {
			drawCross(d, b.center, 0.15, rgba(1, 1, 1, 1))
		}
		if flags&DrawVelocities != 0 && b.typ != Static && b.awake {
			d.Line(b.center, b.center.Add(b.v.Scale(0.1)), rgba(0.3, 1, 0.4, 1))
		}
	}
	if flags&DrawAABBs != 0 {
		for i := range w.shapes.items {
			if !w.shapes.alive[i] || w.shapes.items[i].dead {
				continue
			}
			for _, id := range w.shapes.items[i].proxies {
				drawBox(d, w.tree.fat(id), rgba(0.6, 0.4, 0.8, 0.7))
			}
		}
	}
	if flags&DrawJoints != 0 {
		for i := range w.joints.items {
			if !w.joints.alive[i] {
				continue
			}
			j := &w.joints.items[i]
			ba, bb := &w.bodies.items[j.a.Index], &w.bodies.items[j.b.Index]
			pa, pb := ba.xf.apply(j.localA), bb.xf.apply(j.localB)
			c := rgba(0.5, 0.8, 0.8, 1)
			d.Line(ba.center, pa, c)
			d.Line(pa, pb, c)
			d.Line(pb, bb.center, c)
			if j.kind == JointRevolute {
				d.Circle(pa, 0.08, 8, c)
			}
		}
	}
	if flags&DrawContacts != 0 {
		slop := w.cfg.LinearSlop
		for i := range w.contacts {
			c := &w.contacts[i]
			if !c.alive || c.trigger {
				continue
			}
			for k := 0; k < c.m.Count; k++ {
				p := &c.m.Points[k]
				col := rgba(1, 0.25, 0.25, 1)
				if p.Separation > slop {
					col = rgba(0.3, 0.9, 1, 1)
				}
				drawCross(d, p.Point, 0.1, col)
				d.Line(p.Point, p.Point.Add(c.m.Normal.Scale(0.3)), col)
			}
		}
	}
}
