package editor

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/modules/audio"
	"aego/scene"
	"aego/schema"
)

const (
	minRange = float32(0.1)
	minSize  = float32(0.1)
)

var rectDirs = [8][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}

func snapTo(v, step float32) float32 {
	if step <= 0 {
		return v
	}
	return float32(int(v/step+mathx.Sign(v)*0.5)) * step
}

func sourceAt(s *scene.Scene, id ids.NodeID) (*audio.AudioSource2D, mathx.Vec2, bool) {
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return nil, mathx.Vec2{}, false
	}
	c, ok := scene.Get[audio.AudioSource2D](s, h)
	if !ok || !c.Spatial {
		return nil, mathx.Vec2{}, false
	}
	return c, s.WorldPosition(h), true
}

type rangeGizmo struct {
	td     *schema.TypeDescriptor
	active gizmo.Part
}

func rangeHandles(c *audio.AudioSource2D, at mathx.Vec2) [2]mathx.Vec2 {
	return [2]mathx.Vec2{at.Add(mathx.Vec2{X: c.MinDistance}), at.Add(mathx.Vec2{X: c.MaxDistance})}
}

func (g *rangeGizmo) Draw(c *gizmo.DrawCtx) {
	src, at, ok := sourceAt(c.Scene, c.Node)
	if !ok {
		return
	}
	c.Debug.Circle(at, src.MinDistance, 64, colMin)
	c.Debug.Circle(at, src.MaxDistance, 64, colMax)
	hs := rangeHandles(src, at)
	c.FillHandle(hs[0], 7, c.Color(gizmo.Part(partMin), colMin))
	c.FillHandle(hs[1], 7, c.Color(gizmo.Part(partMax), colMin))
}

func (g *rangeGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	src, at, ok := sourceAt(c.Scene, c.Node)
	if !ok {
		return gizmo.Hit{}, false
	}
	best, bestD := -1, c.Space.Px(9)
	for i, pt := range rangeHandles(src, at) {
		if d := c.Space.ToPixels(pt).Sub(c.MousePx).Len(); d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.Part(partMin + best), Dist: bestD}, true
}

func (g *rangeGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	g.active = hit.Part
}

func (g *rangeGizmo) Update(e *gizmo.EditCtx) {
	src, at, ok := sourceAt(e.Scene, e.Node)
	if !ok {
		return
	}
	r := e.Mouse.Sub(at).Len()
	if e.Snap.Enabled {
		r = snapTo(r, e.Snap.Move)
	}
	name := "maxDistance"
	if g.active == partMin {
		name = "minDistance"
		r = mathx.Clamp(r, 0, max(src.MaxDistance-minRange, 0))
	} else {
		r = max(r, src.MinDistance+minRange)
	}
	if fd, ok := g.td.Field(name); ok {
		gizmo.SetProperty(e, g.td.ID, fd, schema.Float32(r))
	}
}

func (g *rangeGizmo) End(*gizmo.EditCtx) {
	g.active = gizmo.PartNone
}

type zoneGizmo struct {
	td     *schema.TypeDescriptor
	start  mathx.Rect
	active int
}

func zonePoints(r mathx.Rect) [8]mathx.Vec2 {
	var out [8]mathx.Vec2
	cx, cy, hx, hy := r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2
	for i, d := range rectDirs {
		out[i] = mathx.Vec2{X: cx + hx*d[0], Y: cy + hy*d[1]}
	}
	return out
}

func zoneOf(s *scene.Scene, id ids.NodeID) (mathx.Rect, mathx.Mat3, bool) {
	h := s.Handle(id)
	if h.IsZero() || !s.IsAlive(h) {
		return mathx.Rect{}, mathx.Mat3{}, false
	}
	z, ok := scene.Get[audio.AudioZone2D](s, h)
	if !ok {
		return mathx.Rect{}, mathx.Mat3{}, false
	}
	return z.Area, s.World(h), true
}

func (g *zoneGizmo) Draw(c *gizmo.DrawCtx) {
	r, m, ok := zoneOf(c.Scene, c.Node)
	if !ok {
		return
	}
	pts := zonePoints(r)
	for i := 0; i < 4; i++ {
		c.Line(m.Apply(pts[i]), m.Apply(pts[(i+1)%4]), colZone)
	}
	for i, pt := range pts {
		c.FillHandle(m.Apply(pt), 7, c.Color(gizmo.Part(partZone+i), colZone))
	}
}

func (g *zoneGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	r, m, ok := zoneOf(c.Scene, c.Node)
	if !ok {
		return gizmo.Hit{}, false
	}
	best, bestD := -1, c.Space.Px(9)
	for i, pt := range zonePoints(r) {
		if d := c.Space.ToPixels(m.Apply(pt)).Sub(c.MousePx).Len(); d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.Part(partZone + best), Dist: bestD}, true
}

func (g *zoneGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	g.active = int(hit.Part) - partZone
	r, _, ok := zoneOf(e.Scene, e.Node)
	if !ok {
		g.active = -1
		return
	}
	g.start = r
}

func (g *zoneGizmo) Update(e *gizmo.EditCtx) {
	if g.active < 0 || g.active >= len(rectDirs) {
		return
	}
	_, m, ok := zoneOf(e.Scene, e.Node)
	if !ok {
		return
	}
	inv, ok := m.Inverse()
	if !ok {
		return
	}
	lm := inv.Apply(e.Mouse)
	if e.Snap.Enabled {
		lm = mathx.Vec2{X: snapTo(lm.X, e.Snap.Move), Y: snapTo(lm.Y, e.Snap.Move)}
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

func (g *zoneGizmo) End(*gizmo.EditCtx) {
	g.active = -1
}
