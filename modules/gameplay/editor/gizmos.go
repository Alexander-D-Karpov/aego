package editor

import (
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/modules/gameplay"
	"aego/scene"
	"aego/schema"
)

var rectDirs = [8][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}

func handles(r mathx.Rect) [8]mathx.Vec2 {
	var out [8]mathx.Vec2
	cx, cy, hx, hy := r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2
	for i, d := range rectDirs {
		out[i] = mathx.Vec2{X: cx + hx*d[0], Y: cy + hy*d[1]}
	}
	return out
}

func snap(v mathx.Vec2, step float32) mathx.Vec2 {
	if step <= 0 {
		return v
	}
	r := func(f float32) float32 { return float32(int(f/step+mathx.Sign(f)*0.5)) * step }
	return mathx.Vec2{X: r(v.X), Y: r(v.Y)}
}

// drawKnockback draws the push of a hitbox as an arrow, turned with its node.
func drawKnockback(c *gizmo.DrawCtx, s *scene.Scene, h scene.NodeHandle) {
	hb, ok := scene.Get[gameplay.Hitbox2D](s, h)
	if !ok || hb.Knockback == (mathx.Vec2{}) || hb.KnockbackFromCenter {
		return
	}
	m := s.World(h)
	from := m.Apply(mathx.Vec2{X: hb.Area.X + hb.Area.W/2, Y: hb.Area.Y + hb.Area.H/2})
	k := mathx.Vec2{X: m[0]*hb.Knockback.X + m[3]*hb.Knockback.Y, Y: m[1]*hb.Knockback.X + m[4]*hb.Knockback.Y}.Scale(0.15)
	to := from.Add(k)
	c.Line(from, to, colHit)
	if l := k.Len(); l > 0 {
		d := k.Scale(1 / l)
		n := mathx.Vec2{X: -d.Y, Y: d.X}
		back := to.Sub(d.Scale(0.25))
		c.Line(to, back.Add(n.Scale(0.12)), colHit)
		c.Line(to, back.Sub(n.Scale(0.12)), colHit)
	}
}

// areaGizmo edits the Area rectangle of a gameplay component with eight handles.
type areaGizmo struct {
	td     *schema.TypeDescriptor
	col    mathx.Color
	read   func(*scene.Scene, scene.NodeHandle) (mathx.Rect, bool)
	extra  func(*gizmo.DrawCtx, *scene.Scene, scene.NodeHandle)
	start  mathx.Rect
	active int
}

func (g *areaGizmo) Draw(c *gizmo.DrawCtx) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return
	}
	r, ok := g.read(s, h)
	if !ok {
		return
	}
	m := s.World(h)
	pts := handles(r)
	for i := 0; i < 4; i++ {
		c.Line(m.Apply(pts[i]), m.Apply(pts[(i+1)%4]), g.col)
	}
	for i, pt := range pts {
		c.FillHandle(m.Apply(pt), 7, c.Color(gizmo.Part(partBase+i), g.col))
	}
	if g.extra != nil {
		g.extra(c, s, h)
	}
}

func (g *areaGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return gizmo.Hit{}, false
	}
	r, ok := g.read(s, h)
	if !ok {
		return gizmo.Hit{}, false
	}
	m := s.World(h)
	best, bestD := -1, c.Space.Px(9)
	for i, pt := range handles(r) {
		if d := c.Space.ToPixels(m.Apply(pt)).Sub(c.MousePx).Len(); d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.Part(partBase + best), Dist: bestD}, true
}

func (g *areaGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	g.active = int(hit.Part) - partBase
	s := e.Scene
	if r, ok := g.read(s, s.Handle(e.Node)); ok {
		g.start = r
	} else {
		g.active = -1
	}
}

func (g *areaGizmo) Update(e *gizmo.EditCtx) {
	if g.active < 0 || g.active >= len(rectDirs) {
		return
	}
	s := e.Scene
	h := s.Handle(e.Node)
	inv, ok := s.World(h).Inverse()
	if h.IsZero() || !ok {
		return
	}
	lm := inv.Apply(e.Mouse)
	if e.Snap.Enabled {
		lm = snap(lm, e.Snap.Move)
	}
	st := g.start
	lo := mathx.Vec2{X: st.X, Y: st.Y}
	hi := mathx.Vec2{X: st.X + st.W, Y: st.Y + st.H}
	d := rectDirs[g.active]
	switch {
	case d[0] > 0:
		hi.X = max(lm.X, lo.X+minSize)
	case d[0] < 0:
		lo.X = min(lm.X, hi.X-minSize)
	}
	switch {
	case d[1] > 0:
		hi.Y = max(lm.Y, lo.Y+minSize)
	case d[1] < 0:
		lo.Y = min(lm.Y, hi.Y-minSize)
	}
	if fd, ok := g.td.Field("area"); ok {
		gizmo.SetProperty(e, g.td.ID, fd, schema.Rect(mathx.Rect{X: lo.X, Y: lo.Y, W: hi.X - lo.X, H: hi.Y - lo.Y}))
	}
}

func (g *areaGizmo) End(*gizmo.EditCtx) {
	g.active = -1
}
