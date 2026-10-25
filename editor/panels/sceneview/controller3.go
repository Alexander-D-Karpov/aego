package sceneview

import (
	"math"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/editor/core/props"
	"aego/editor/core/shape"
	"aego/render/debug"
	"aego/render/view"
	"aego/scene"
)

type Host3 interface {
	Scene() *scene.Scene
	Shapes() shape.Set
	Selected() []ids.NodeID
	Select(nodes []ids.NodeID, additive bool)
	Commit3D(label string, edits []props.Transform3DEdit)
	Tool3() gizmo.Op3
	ShowGizmo() bool
	LocalAxes() bool
	Snap3() gizmo.Snap3
}

const (
	ButtonLeft = iota
	ButtonRight
	ButtonMiddle
)

const (
	clickSlopPx float32 = 4
	pointPickPx float32 = 10
	axesSizePx  float32 = 80
	axesMargin  float32 = 8
)

type Input3 struct {
	Mouse   mathx.Vec2
	Delta   mathx.Vec2
	Wheel   float32
	Down    [3]bool
	Pressed [3]bool
	Hovered bool
	Alt     bool
	Shift   bool
	Frame   bool
	Cancel  bool
	// Scale is pixels per UI unit; the click slop is in UI units, so HiDPI screens get the same feel.
	Scale float32
}

func (in Input3) slop() float32 {
	return clickSlopPx * max(in.Scale, 1)
}

type Theme3 struct {
	Selection mathx.Color
	Camera    mathx.Color
	Box       mathx.Color
	Grid      mathx.Color
	Gizmo     gizmo.Colors3
}

type nav3 uint8

const (
	navNone nav3 = iota
	navOrbit
	navPan
)

type edit3 struct {
	pressAt mathx.Vec2
	moved   bool
	drag    gizmo.Drag3
	nodes   []ids.NodeID
	parents []mathx.Mat4
	before  []scene.Transform3D
}

type Controller3 struct {
	Cam      Camera3D
	vp       mathx.Rect
	hot      gizmo.Part3
	nav      nav3
	edit     *edit3
	pressing bool
	boxing   bool
	pressAt  mathx.Vec2
	mouse    mathx.Vec2
	mask     uint64
}

func NewController3() *Controller3 {
	return &Controller3{Cam: DefaultCamera3D(), vp: mathx.Rect{W: 1, H: 1}, mask: ^uint64(0)}
}

func (c *Controller3) SetViewport(w, h float32) {
	c.vp = mathx.Rect{W: max(w, 1), H: max(h, 1)}
}

func (c *Controller3) View() view.View {
	return c.Cam.View(c.vp)
}

func (c *Controller3) Ray(p mathx.Vec2) mathx.Ray3 {
	return c.View().ScreenToRay(p)
}

func (c *Controller3) Hot() gizmo.Part3 {
	return c.hot
}

func (c *Controller3) Busy() bool {
	return c.edit != nil || c.nav != navNone || c.pressing
}

func ancestorIn(s *scene.Scene, h scene.NodeHandle, set []ids.NodeID) bool {
	for p := s.Parent(h); !p.IsZero() && p != s.Root(); p = s.Parent(p) {
		id := s.ID(p)
		for _, x := range set {
			if x == id {
				return true
			}
		}
	}
	return false
}

func editable(s *scene.Scene, selected []ids.NodeID) []ids.NodeID {
	out := make([]ids.NodeID, 0, len(selected))
	for _, id := range selected {
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) || s.Space(h) != scene.Space3D || ancestorIn(s, h, selected) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (c *Controller3) gizmoFrame(h Host3) (gizmo.Frame3, bool) {
	if !h.ShowGizmo() {
		return gizmo.Frame3{}, false
	}
	s := h.Scene()
	nodes := editable(s, h.Selected())
	if len(nodes) == 0 {
		return gizmo.Frame3{}, false
	}
	var sum mathx.Vec3
	for _, id := range nodes {
		sum = sum.Add(s.WorldPosition3D(s.Handle(id)))
	}
	f := gizmo.Frame3{View: c.View(), Pivot: sum.Scale(1 / float32(len(nodes))), Rotation: mathx.QuatIdentity()}
	if h.LocalAxes() || h.Tool3() == gizmo.Scale3 {
		if _, rot, _, ok := s.World4(s.Handle(nodes[len(nodes)-1])).Decompose(); ok {
			f.Rotation = rot
		}
	}
	return f, true
}

func (c *Controller3) Update(h Host3, in Input3) bool {
	c.mouse = in.Mouse
	if c.edit != nil {
		return c.updateEdit(h, in)
	}
	if c.nav != navNone {
		if !in.Down[ButtonLeft] && !in.Down[ButtonRight] && !in.Down[ButtonMiddle] {
			c.nav = navNone
			return true
		}
		if c.nav == navOrbit {
			c.Cam.Orbit(in.Delta.X, in.Delta.Y)
		} else {
			c.Cam.Pan(in.Delta.X, in.Delta.Y, c.vp.H)
		}
		return true
	}
	if c.pressing {
		return c.updatePress(h, in)
	}
	if !in.Hovered {
		c.hot = gizmo.Part3None
		return false
	}
	consumed := false
	if in.Wheel != 0 {
		c.Cam.Dolly(in.Wheel)
		consumed = true
	}
	if in.Frame {
		c.FrameSelection(h)
		consumed = true
	}
	switch {
	case in.Pressed[ButtonRight] || (in.Pressed[ButtonLeft] && in.Alt):
		c.nav = navOrbit
		return true
	case in.Pressed[ButtonMiddle]:
		c.nav = navPan
		return true
	}
	frame, ok := c.gizmoFrame(h)
	c.hot = gizmo.Part3None
	if ok {
		c.hot = gizmo.Hit3(h.Tool3(), frame, c.Ray(in.Mouse))
	}
	if in.Pressed[ButtonLeft] {
		if c.hot != gizmo.Part3None && c.beginEdit(h, frame, in) {
			return true
		}
		c.pressing = true
		c.pressAt = in.Mouse
		return true
	}
	return consumed
}

func (c *Controller3) beginEdit(h Host3, f gizmo.Frame3, in Input3) bool {
	d, ok := gizmo.BeginDrag3(h.Tool3(), c.hot, f, c.Ray(in.Mouse))
	if !ok {
		return false
	}
	s := h.Scene()
	e := &edit3{drag: d, pressAt: in.Mouse}
	for _, id := range editable(s, h.Selected()) {
		n := s.Handle(id)
		e.nodes = append(e.nodes, id)
		e.parents = append(e.parents, s.World4(s.Parent(n)))
		e.before = append(e.before, s.Local3D(n))
	}
	c.edit = e
	return true
}

func (c *Controller3) restore(s *scene.Scene) {
	for i, id := range c.edit.nodes {
		if n := s.Handle(id); !n.IsZero() {
			s.SetLocal3D(n, c.edit.before[i])
		}
	}
}

func (c *Controller3) updateEdit(h Host3, in Input3) bool {
	e := c.edit
	s := h.Scene()
	if in.Cancel {
		c.restore(s)
		c.edit = nil
		return true
	}
	if !e.moved {
		dx, dy := in.Mouse.X-e.pressAt.X, in.Mouse.Y-e.pressAt.Y
		if mathx.Sqrt(dx*dx+dy*dy) <= in.slop() {
			if !in.Down[ButtonLeft] {
				c.edit = nil
			}
			return true
		}
		e.moved = true
	}
	if delta, ok := e.drag.Update(c.Ray(in.Mouse), h.Snap3()); ok {
		for i, id := range e.nodes {
			if n := s.Handle(id); !n.IsZero() {
				s.SetLocal3D(n, gizmo.ApplyDelta3(e.parents[i], e.before[i], e.drag.Pivot, delta))
			}
		}
	}
	if in.Down[ButtonLeft] {
		return true
	}
	edits := make([]props.Transform3DEdit, 0, len(e.nodes))
	for i, id := range e.nodes {
		n := s.Handle(id)
		if n.IsZero() {
			continue
		}
		after := s.Local3D(n)
		if after != e.before[i] {
			edits = append(edits, props.Transform3DEdit{Node: id, Before: e.before[i], After: after})
		}
	}
	c.restore(s)
	c.edit = nil
	if len(edits) > 0 {
		h.Commit3D(e.drag.Op.Label(), edits)
	}
	return true
}

func (c *Controller3) updatePress(h Host3, in Input3) bool {
	dx, dy := in.Mouse.X-c.pressAt.X, in.Mouse.Y-c.pressAt.Y
	if !c.boxing && mathx.Sqrt(dx*dx+dy*dy) > in.slop() {
		c.boxing = true
	}
	if in.Cancel {
		c.pressing, c.boxing = false, false
		return true
	}
	if in.Down[ButtonLeft] {
		return true
	}
	c.pressing = false
	if c.boxing {
		c.boxing = false
		h.Select(BoxSelect3D(h.Scene(), h.Shapes(), c.View(), c.pressAt, in.Mouse, c.mask), in.Shift)
		return true
	}
	if n, ok := c.PickAt(h, in.Mouse); ok {
		h.Select([]ids.NodeID{h.Scene().ID(n)}, in.Shift)
	} else if !in.Shift {
		h.Select(nil, false)
	}
	return true
}

func (c *Controller3) PickAt(h Host3, p mathx.Vec2) (scene.NodeHandle, bool) {
	s := h.Scene()
	ray := c.Ray(p)
	v := c.View()
	best, bestT, ok := Pick3D(s, h.Shapes(), ray, c.mask)
	radius := func(at mathx.Vec3) float32 {
		return gizmo.PxToWorld(v, at, pointPickPx)
	}
	if pt, t, pok := PickPoint3D(s, h.Shapes(), ray, c.mask, radius); pok && (!ok || t < bestT) {
		return pt, true
	}
	return best, ok
}

func (c *Controller3) FrameSelection(h Host3) {
	s := h.Scene()
	b, ok := SelectionBounds(s, h.Shapes(), h.Selected())
	if !ok {
		b, ok = SceneBounds(s, h.Shapes())
	}
	if !ok {
		b = mathx.AABB3{Min: mathx.Vec3{X: -1, Y: -1, Z: -1}, Max: mathx.Vec3{X: 1, Y: 1, Z: 1}}
	}
	c.Cam.Frame(b)
}

func (c *Controller3) DropPoint(h Host3, p mathx.Vec2, snap float32) mathx.Vec3 {
	ray := c.Ray(p)
	var at mathx.Vec3
	if _, t, ok := Pick3D(h.Scene(), h.Shapes(), ray, c.mask); ok {
		at = ray.At(t)
	} else if t, ok := intersectPlane(ray, mathx.Vec3{}, mathx.Vec3{Y: 1}); ok {
		at = ray.At(t)
	} else {
		at = c.Cam.Target
	}
	if snap > 0 {
		at.X = snapTo(at.X, snap)
		at.Z = snapTo(at.Z, snap)
	}
	return at
}

func snapTo(v, step float32) float32 {
	if step <= 0 {
		return v
	}
	return float32(math.Round(float64(v/step))) * step
}

func (c *Controller3) AxesView(base view.View) view.View {
	cam := Camera3D{Distance: 4, Yaw: c.Cam.Yaw, Pitch: c.Cam.Pitch, FOV: 30}
	v := cam.View(mathx.Rect{X: axesMargin, Y: c.vp.H - axesSizePx - axesMargin, W: axesSizePx, H: axesSizePx})
	v.Target, v.TargetW, v.TargetH = base.Target, base.TargetW, base.TargetH
	v.PixelScale = base.PixelScale
	v.DoClear = false
	v.Label = "sceneview3d.axes"
	return v
}

func DrawGrid3(dbg *debug.Draw, cam Camera3D, step float32, th Theme3) {
	if step <= 0 {
		step = 1
	}
	for cam.dist()/step > 40 {
		step *= 10
	}
	const count = 50
	center := mathx.Vec3{X: snapTo(cam.Target.X, step*10), Z: snapTo(cam.Target.Z, step*10)}
	dbg.Grid3(center, step, count, th.Grid)
	ext := step * count
	dbg.Line3(mathx.Vec3{X: center.X - ext}, mathx.Vec3{X: center.X + ext}, th.Gizmo.X)
	dbg.Line3(mathx.Vec3{Z: center.Z - ext}, mathx.Vec3{Z: center.Z + ext}, th.Gizmo.Z)
}

func (c *Controller3) DrawOverlay(h Host3, dbg *debug.Draw, th Theme3) {
	s := h.Scene()
	v := c.View()
	dbg.Space(debug.Overlay)
	sh := h.Shapes()
	sh.Overlays(&shape.OverlayCtx{Scene: s, Debug: dbg, View: v, Mode3D: true, Aspect: c.vp.W / c.vp.H, Color: th.Camera})
	for _, id := range h.Selected() {
		n := s.Handle(id)
		if n.IsZero() || !s.IsAlive(n) {
			continue
		}
		if b, ok := sh.Of(s, n); ok && b.Kind == shape.Box {
			dbg.AABB3(b.Box, th.Selection)
			continue
		}
		dbg.Axes3(s.World4(n), gizmo.PxToWorld(v, s.WorldPosition3D(n), 40))
	}
	if f, ok := c.gizmoFrame(h); ok {
		dc := &gizmo.DrawCtx3{Debug: dbg, Frame: f, Colors: th.Gizmo, Hot: c.hot}
		if c.edit != nil {
			dc.Active = c.edit.drag.Part
		}
		gizmo.Draw3(h.Tool3(), dc)
	}
	if c.boxing {
		dbg.Space(debug.Screen)
		dbg.AABB(mathx.AABB2{
			Min: mathx.Vec2{X: min(c.pressAt.X, c.mouse.X), Y: min(c.pressAt.Y, c.mouse.Y)},
			Max: mathx.Vec2{X: max(c.pressAt.X, c.mouse.X), Y: max(c.pressAt.Y, c.mouse.Y)},
		}, th.Box)
	}
}
