package editor

import (
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/modules/camera"
	"aego/scene"
	"aego/schema"
)

const minSize = float32(0.1)

var rectDirs = [8][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}

func handlePoints(r mathx.Rect) [8]mathx.Vec2 {
	var out [8]mathx.Vec2
	cx, cy, hx, hy := r.X+r.W/2, r.Y+r.H/2, r.W/2, r.H/2
	for i, d := range rectDirs {
		out[i] = mathx.Vec2{X: cx + hx*d[0], Y: cy + hy*d[1]}
	}
	return out
}

func snapVec(v mathx.Vec2, step float32) mathx.Vec2 {
	if step <= 0 {
		return v
	}
	r := func(f float32) float32 { return float32(int(f/step+mathx.Sign(f)*0.5)) * step }
	return mathx.Vec2{X: r(v.X), Y: r(v.Y)}
}

func readBounds(s *scene.Scene, h scene.NodeHandle) (mathx.Rect, bool) {
	vc, ok := scene.Get[camera.VirtualCamera2D](s, h)
	if !ok || !vc.UseBounds {
		return mathx.Rect{}, false
	}
	return vc.Bounds, true
}

func readArea(s *scene.Scene, h scene.NodeHandle) (mathx.Rect, bool) {
	z, ok := scene.Get[camera.CameraZone2D](s, h)
	if !ok {
		return mathx.Rect{}, false
	}
	return z.Area, true
}

func drawFrame(c *gizmo.DrawCtx, s *scene.Scene, h scene.NodeHandle) {
	if scene.Has[camera.CameraFollow2D](s, h) {
		return
	}
	c.Debug.Rect(camera.Frame(s.WorldPosition(h), editorSize(s, h)), colFrame)
}

type rectGizmo struct {
	td     *schema.TypeDescriptor
	field  string
	local  bool
	col    mathx.Color
	read   func(*scene.Scene, scene.NodeHandle) (mathx.Rect, bool)
	extra  func(*gizmo.DrawCtx, *scene.Scene, scene.NodeHandle)
	start  mathx.Rect
	active int
}

func (g *rectGizmo) matrix(s *scene.Scene, h scene.NodeHandle) mathx.Mat3 {
	if g.local {
		return s.World(h)
	}
	return mathx.Mat3Identity()
}

func (g *rectGizmo) Draw(c *gizmo.DrawCtx) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return
	}
	if g.extra != nil {
		g.extra(c, s, h)
	}
	r, ok := g.read(s, h)
	if !ok {
		return
	}
	m := g.matrix(s, h)
	pts := handlePoints(r)
	for i := 0; i < 4; i++ {
		c.Line(m.Apply(pts[i]), m.Apply(pts[(i+1)%4]), g.col)
	}
	for i, pt := range pts {
		c.FillHandle(m.Apply(pt), 7, c.Color(gizmo.Part(partBase+i), g.col))
	}
}

func (g *rectGizmo) HitTest(c *gizmo.HitCtx) (gizmo.Hit, bool) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return gizmo.Hit{}, false
	}
	r, ok := g.read(s, h)
	if !ok {
		return gizmo.Hit{}, false
	}
	m := g.matrix(s, h)
	best, bestD := -1, c.Space.Px(9)
	for i, pt := range handlePoints(r) {
		if d := c.Space.ToPixels(m.Apply(pt)).Sub(c.MousePx).Len(); d <= bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return gizmo.Hit{}, false
	}
	return gizmo.Hit{Part: gizmo.Part(partBase + best), Dist: bestD}, true
}

func (g *rectGizmo) Begin(e *gizmo.EditCtx, hit gizmo.Hit) {
	e.Part = hit.Part
	g.active = int(hit.Part) - partBase
	s := e.Scene
	r, ok := g.read(s, s.Handle(e.Node))
	if !ok {
		g.active = -1
		return
	}
	g.start = r
}

func (g *rectGizmo) Update(e *gizmo.EditCtx) {
	if g.active < 0 || g.active >= len(rectDirs) {
		return
	}
	s := e.Scene
	h := s.Handle(e.Node)
	if h.IsZero() {
		return
	}
	inv, ok := g.matrix(s, h).Inverse()
	if !ok {
		return
	}
	lm := inv.Apply(e.Mouse)
	if e.Snap.Enabled {
		lm = snapVec(lm, e.Snap.Move)
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
	if fd, ok := g.td.Field(g.field); ok {
		gizmo.SetProperty(e, g.td.ID, fd, schema.Rect(mathx.Rect{X: lo.X, Y: lo.Y, W: hi.X - lo.X, H: hi.Y - lo.Y}))
	}
}

func (g *rectGizmo) End(*gizmo.EditCtx) {}

type followGizmo struct{}

func (followGizmo) Draw(c *gizmo.DrawCtx) {
	s := c.Scene
	h := s.Handle(c.Node)
	if h.IsZero() || !s.IsAlive(h) {
		return
	}
	f, ok := scene.Get[camera.CameraFollow2D](s, h)
	if !ok {
		return
	}
	pos := s.WorldPosition(h)
	size := editorSize(s, h)
	c.Debug.Rect(camera.Frame(pos, size), colFrame)
	c.Debug.Rect(camera.DeadzoneRect(pos, size, f.Deadzone), colDeadzone)
	if th, ok := f.Target.Resolve(s); ok && s.IsAlive(th) {
		c.Line(pos, s.WorldPosition(th), colDim)
	}
}

func (followGizmo) HitTest(*gizmo.HitCtx) (gizmo.Hit, bool) {
	return gizmo.Hit{}, false
}

func (followGizmo) Begin(*gizmo.EditCtx, gizmo.Hit) {}

func (followGizmo) Update(*gizmo.EditCtx) {}

func (followGizmo) End(*gizmo.EditCtx) {}
