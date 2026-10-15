package editor

import (
	"slices"

	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/modules/physics/components"
	"aego/platform/input"
	"aego/scene"
	"aego/schema"
)

const minSize = float32(0.01)

var (
	colBad    = mathx.Color{R: 1, G: 0.3, B: 0.3, A: 1}
	colInsert = mathx.Color{R: 0.8, G: 0.8, B: 0.8, A: 0.7}
)

func (p *plugin) mods() input.Mods {
	if p == nil || p.ed == nil || p.ed.UI == nil {
		return 0
	}
	return p.ed.UI.Mods()
}

func snap(v mathx.Vec2, step float32) mathx.Vec2 {
	if step <= 0 {
		return v
	}
	r := func(f float32) float32 { return float32(int(f/step+mathx.Sign(f)*0.5)) * step }
	return mathx.Vec2{X: r(v.X), Y: r(v.Y)}
}

func pointsValue(pts []mathx.Vec2) schema.Value {
	list := make([]schema.Value, len(pts))
	for i, pt := range pts {
		list[i] = schema.Vec2(pt)
	}
	return schema.Value{Kind: schema.KindList, List: list}
}

type colliderGizmo struct {
	p      *plugin
	kind   components.Kind
	td     *schema.TypeDescriptor
	list   []handle
	col    components.Collider
	start  components.Collider
	pts    []mathx.Vec2
	work   []mathx.Vec2
	line   []mathx.Vec2
	active handle
	done   bool
}

func (g *colliderGizmo) set(e *gizmo.EditCtx, name string, v schema.Value) {
	if fd, ok := g.td.Field(name); ok {
		gizmo.SetProperty(e, g.td.ID, fd, v)
	}
}

func (g *colliderGizmo) Draw(c *gizmo.DrawCtx) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !components.Read(s, h, g.kind, &g.col) {
		return
	}
	m := s.World(h)
	var closed bool
	g.line, closed = outline(&g.col, g.line)
	col := c.Colors.Rotate
	if g.kind == components.KindPolygon && !convex(g.line) {
		col = colBad
	}
	drawPoly(c.Debug, m, g.line, closed, col)
	g.list = handles(&g.col, g.list[:0])
	for i, hd := range g.list {
		at := m.Apply(hd.at)
		part := gizmo.Part(partBase + i)
		if hd.kind == hInsert {
			c.Handle(at, 6, c.Color(part, colInsert))
			continue
		}
		c.FillHandle(at, 8, c.Color(part, col))
	}
}

func (g *colliderGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !components.Read(s, h, g.kind, &g.col) {
		return gizmo.Hit{}, false
	}
	m := s.World(h)
	g.list = handles(&g.col, g.list[:0])
	tol := c.Space.Px(9)
	best, bestD := -1, tol
	for pass := hMove; pass <= hInsert && best < 0; pass++ {
		for i, hd := range g.list {
			if hd.kind != pass {
				continue
			}
			if d := c.Space.ToPixels(m.Apply(hd.at)).Sub(c.MousePx).Len(); d <= bestD {
				best, bestD = i, d
			}
		}
	}
	if best < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.Part(partBase + best), Dist: bestD}, true
}

func (g *colliderGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	g.done = true
	s := e.Scene
	h := s.Handle(e.Node)
	if h.IsZero() || !components.Read(s, h, g.kind, &g.col) {
		return
	}
	g.start = g.col
	g.pts = append(g.pts[:0], g.col.Points...)
	g.start.Points = g.pts
	g.list = handles(&g.start, g.list[:0])
	i := int(hit.Part) - partBase
	if i < 0 || i >= len(g.list) {
		return
	}
	g.active, g.done = g.list[i], false
	if g.kind != components.KindPolygon && g.kind != components.KindEdge {
		return
	}
	minPts := 3
	if g.kind == components.KindEdge {
		minPts = 2
	}
	switch {
	case g.active.kind == hMove && g.p.mods().Has(input.ModAlt):
		if len(g.pts) > minPts {
			g.pts = slices.Delete(g.pts, g.active.index, g.active.index+1)
			g.set(e, "points", pointsValue(g.pts))
		}
		g.done = true
	case g.active.kind == hInsert:
		i := g.active.index
		mid := g.pts[i].Lerp(g.pts[(i+1)%len(g.pts)], 0.5)
		g.pts = slices.Insert(g.pts, i+1, mid)
		g.start.Points = g.pts
		g.set(e, "points", pointsValue(g.pts))
		g.active = handle{kind: hMove, index: i + 1}
	}
}

func (g *colliderGizmo) Update(e *gizmo.EditCtx) {
	if g.done {
		return
	}
	s := e.Scene
	h := s.Handle(e.Node)
	if h.IsZero() {
		return
	}
	inv, ok := s.World(h).Inverse()
	if !ok {
		return
	}
	lm := inv.Apply(e.Mouse)
	if e.Snap.Enabled {
		lm = snap(lm, e.Snap.Move)
	}
	st := &g.start
	o := st.Common.Offset
	a := g.active
	switch g.kind {
	case components.KindBox:
		lo, hi := o.Sub(st.Size.Scale(0.5)), o.Add(st.Size.Scale(0.5))
		switch {
		case a.nx > 0:
			hi.X = max(lm.X, lo.X+minSize)
		case a.nx < 0:
			lo.X = min(lm.X, hi.X-minSize)
		}
		switch {
		case a.ny > 0:
			hi.Y = max(lm.Y, lo.Y+minSize)
		case a.ny < 0:
			lo.Y = min(lm.Y, hi.Y-minSize)
		}
		g.set(e, "offset", schema.Vec2(lo.Add(hi).Scale(0.5)))
		g.set(e, "size", schema.Vec2(hi.Sub(lo)))
	case components.KindCircle:
		g.set(e, "radius", schema.Float32(max(lm.Sub(o).Len(), minSize)))
	case components.KindCapsule:
		if a.index == 2 {
			g.set(e, "radius", schema.Float32(max(mathx.Abs(lm.Y-o.Y), minSize)))
			return
		}
		g.set(e, "length", schema.Float32(max(2*mathx.Abs(lm.X-o.X), minSize)))
	case components.KindPolygon, components.KindEdge:
		g.work = append(g.work[:0], g.pts...)
		if a.index < len(g.work) {
			g.work[a.index] = lm.Sub(o)
		}
		g.set(e, "points", pointsValue(g.work))
	}
}

func (g *colliderGizmo) End(*gizmo.EditCtx) {}

type jointGizmo struct {
	p        *plugin
	td       *schema.TypeDescriptor
	revolute bool
	active   int
}

func (g *jointGizmo) anchors(s *scene.Scene, h scene.NodeHandle) (a, b mathx.Vec2, target scene.NodeHandle, ok bool) {
	m := s.World(h)
	if g.revolute {
		c, has := scene.Get[components.RevoluteJoint2D](s, h)
		if !has {
			return a, b, target, false
		}
		a = m.Apply(c.Anchor)
		if th := s.Handle(c.Target.NodeRefID()); !th.IsZero() && s.IsAlive(th) {
			target, b = th, s.WorldPosition(th)
		}
		return a, b, target, true
	}
	c, has := scene.Get[components.DistanceJoint2D](s, h)
	if !has {
		return a, b, target, false
	}
	a = m.Apply(c.AnchorA)
	if th := s.Handle(c.Target.NodeRefID()); !th.IsZero() && s.IsAlive(th) {
		target, b = th, s.World(th).Apply(c.AnchorB)
	}
	return a, b, target, true
}

func (g *jointGizmo) Draw(c *gizmo.DrawCtx) {
	s := c.Scene
	h := s.Handle(c.Node)
	a, b, target, ok := g.anchors(s, h)
	if !ok {
		return
	}
	col := c.Colors.Rotate
	c.Line(s.WorldPosition(h), a, col)
	if !target.IsZero() {
		c.Line(a, b, col)
		if !g.revolute {
			c.FillHandle(b, 8, c.Color(gizmo.Part(partBase+1), col))
		}
	}
	if g.revolute {
		c.Circle(a, 10, col)
	}
	c.FillHandle(a, 8, c.Color(gizmo.Part(partBase), col))
}

func (g *jointGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	s := c.Scene
	a, b, target, ok := g.anchors(s, s.Handle(c.Node))
	if !ok {
		return gizmo.Hit{}, false
	}
	tol := c.Space.Px(9)
	if d := c.Space.ToPixels(a).Sub(c.MousePx).Len(); d <= tol {
		return gizmo.Hit{Part: partBase, Dist: d}, true
	}
	if !g.revolute && !target.IsZero() {
		if d := c.Space.ToPixels(b).Sub(c.MousePx).Len(); d <= tol {
			return gizmo.Hit{Part: partBase + 1, Dist: d}, true
		}
	}
	return gizmo.Hit{}, false
}

func (g *jointGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	g.active = int(hit.Part) - partBase
}

func (g *jointGizmo) Update(e *gizmo.EditCtx) {
	s := e.Scene
	h := s.Handle(e.Node)
	_, _, target, ok := g.anchors(s, h)
	if !ok {
		return
	}
	at := e.Mouse
	if e.Snap.Enabled {
		at = snap(at, e.Snap.Move)
	}
	node, name := h, "anchorA"
	switch {
	case g.revolute:
		name = "anchor"
	case g.active == 1:
		if target.IsZero() {
			return
		}
		node, name = target, "anchorB"
	}
	inv, ok := s.World(node).Inverse()
	if !ok {
		return
	}
	if fd, ok := g.td.Field(name); ok {
		gizmo.SetProperty(e, g.td.ID, fd, schema.Vec2(inv.Apply(at)))
	}
}

func (g *jointGizmo) End(*gizmo.EditCtx) {}
