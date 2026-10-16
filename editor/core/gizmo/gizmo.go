package gizmo

import (
	"math"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/render/debug"
	"aego/render/view"
	"aego/scene"
)

type Part uint8

const (
	PartNone Part = iota
	PartX
	PartY
	PartXY
	PartRotate
	PartScaleX
	PartScaleY
	PartScaleXY
)

type Hit struct {
	Part Part
	Dist float32
}

type Snap struct {
	Enabled bool
	Move    float32
	Rotate  float32
	Scale   float32
}

type Space struct {
	View  view.View
	Zoom  float32
	Scale float32
}

func (s Space) PxToWorld(px float32) float32 {
	if s.Zoom <= 0 {
		return px
	}
	return px / s.Zoom
}

func (s Space) Px(px float32) float32 {
	if s.Scale <= 0 {
		return px
	}
	return px * s.Scale
}

func (s Space) ToPixels(w mathx.Vec2) mathx.Vec2 {
	return s.View.WorldToScreen(w)
}

type Colors struct {
	X      mathx.Color
	Y      mathx.Color
	Center mathx.Color
	Rotate mathx.Color
	Hot    mathx.Color
}

type DrawCtx struct {
	Debug    *debug.Draw
	Space    Space
	Scene    *scene.Scene
	Node     ids.NodeID
	Pivot    mathx.Vec2
	Rotation float32
	Hot      Part
	Active   Part
	Colors   Colors
}

func (c *DrawCtx) Color(p Part, base mathx.Color) mathx.Color {
	if c.Active == p || (c.Active == PartNone && c.Hot == p) {
		return c.Colors.Hot
	}
	return base
}

func (c *DrawCtx) PxToWorld(px float32) float32 {
	return c.Space.PxToWorld(px)
}

func (c *DrawCtx) Line(a, b mathx.Vec2, col mathx.Color) {
	c.Debug.Line(a, b, col)
}

func (c *DrawCtx) Arrow(a, b mathx.Vec2, col mathx.Color) {
	c.Debug.Arrow(a, b, col)
}

func (c *DrawCtx) Circle(center mathx.Vec2, rPx float32, col mathx.Color) {
	c.Debug.Circle(center, c.PxToWorld(rPx), 48, col)
}

func (c *DrawCtx) Handle(center mathx.Vec2, sizePx float32, col mathx.Color) {
	h := c.PxToWorld(sizePx) / 2
	c.Debug.Rect(mathx.Rect{X: center.X - h, Y: center.Y - h, W: 2 * h, H: 2 * h}, col)
}

func (c *DrawCtx) Thick(a, b mathx.Vec2, col mathx.Color, px float32) {
	d := b.Sub(a)
	l := d.Len()
	if l == 0 {
		return
	}
	n := mathx.Vec2{X: -d.Y / l, Y: d.X / l}
	k := max(1, int(px/0.75))
	step := c.PxToWorld(0.75)
	for i := 0; i < k; i++ {
		o := n.Scale((float32(i) - float32(k-1)/2) * step)
		c.Debug.Line(a.Add(o), b.Add(o), col)
	}
}

func (c *DrawCtx) Triangle(tip, dir mathx.Vec2, lenPx, widthPx float32, col mathx.Color) {
	l := dir.Len()
	if l == 0 {
		return
	}
	dir = dir.Scale(1 / l)
	n := mathx.Vec2{X: -dir.Y, Y: dir.X}
	base := tip.Sub(dir.Scale(c.PxToWorld(lenPx)))
	half := c.PxToWorld(widthPx) / 2
	steps := int(widthPx/0.75) + 1
	for i := 0; i <= steps; i++ {
		t := float32(i)/float32(steps)*2 - 1
		c.Debug.Line(tip, base.Add(n.Scale(half*t)), col)
	}
}

func (c *DrawCtx) FillHandle(center mathx.Vec2, sizePx float32, col mathx.Color) {
	h := c.PxToWorld(sizePx) / 2
	step := c.PxToWorld(0.75)
	for y := -h; y <= h; y += step {
		c.Debug.Line(mathx.Vec2{X: center.X - h, Y: center.Y + y}, mathx.Vec2{X: center.X + h, Y: center.Y + y}, col)
	}
}

func (c *DrawCtx) Ring(center mathx.Vec2, rPx, px float32, col mathx.Color) {
	k := max(1, int(px/0.75))
	for i := 0; i < k; i++ {
		r := rPx + (float32(i)-float32(k-1)/2)*0.75
		c.Debug.Circle(center, c.PxToWorld(r), 64, col)
	}
}

type HitCtx struct {
	Space    Space
	Scene    *scene.Scene
	Node     ids.NodeID
	Pivot    mathx.Vec2
	Rotation float32
	MousePx  mathx.Vec2
}

type Target struct {
	Node       ids.NodeID
	Start      scene.Transform2D
	StartWorld mathx.Vec2
	ParentInv  mathx.Mat3
}

type EditCtx struct {
	Scene    *scene.Scene
	Tx       *history.Transaction
	Node     ids.NodeID
	Targets  []Target
	Pivot    mathx.Vec2
	Rotation float32
	AxisX    mathx.Vec2
	AxisY    mathx.Vec2
	Start    mathx.Vec2
	Mouse    mathx.Vec2
	Snap     Snap
	Part     Part
	Err      error
}

type Gizmo interface {
	Draw(c *DrawCtx)
	HitTest(c *HitCtx) (Hit, bool)
	Begin(e *EditCtx, h Hit)
	Update(e *EditCtx)
	End(e *EditCtx)
}

func NewEdit(s *scene.Scene, nodes []ids.NodeID, active ids.NodeID, start mathx.Vec2, snap Snap) *EditCtx {
	s.UpdateTransforms()
	e := &EditCtx{Scene: s, Node: active, Start: start, Mouse: start, Snap: snap}
	for _, id := range nodes {
		h := s.Handle(id)
		if h.IsZero() || !s.IsAlive(h) {
			continue
		}
		inv, ok := s.World(s.Parent(h)).Inverse()
		if !ok {
			continue
		}
		e.Targets = append(e.Targets, Target{
			Node:       id,
			Start:      s.Local(h),
			StartWorld: s.WorldPosition(h),
			ParentInv:  inv,
		})
	}
	if ah := s.Handle(active); !ah.IsZero() && s.IsAlive(ah) {
		m := s.World(ah)
		e.Pivot = mathx.Vec2{X: m[6], Y: m[7]}
		e.Rotation = mathx.Atan2(m[1], m[0])
	} else if len(e.Targets) > 0 {
		e.Pivot = e.Targets[0].StartWorld
	}
	return e
}

func (e *EditCtx) set(i int, t scene.Transform2D) {
	if h := e.Scene.Handle(e.Targets[i].Node); !h.IsZero() {
		e.Scene.SetLocal(h, t)
	}
}

func Cancel(e *EditCtx) {
	for i := range e.Targets {
		e.set(i, e.Targets[i].Start)
	}
}

func Commit(e *EditCtx) {
	if e.Tx == nil {
		return
	}
	for _, t := range e.Targets {
		h := e.Scene.Handle(t.Node)
		if h.IsZero() {
			continue
		}
		cur := e.Scene.Local(h)
		if cur == t.Start {
			continue
		}
		e.Scene.SetLocal(h, t.Start)
		if err := e.Tx.Exec(&commands.SetTransform{Node: t.Node, Before: t.Start, After: cur}); err != nil {
			e.Err = err
			Cancel(e)
			return
		}
	}
}

func segDist(p, a, b mathx.Vec2) float32 {
	ab := b.Sub(a)
	l := ab.LenSq()
	if l == 0 {
		return p.Sub(a).Len()
	}
	t := mathx.Clamp(p.Sub(a).Dot(ab)/l, 0, 1)
	return p.Sub(a.Add(ab.Scale(t))).Len()
}

func snapTo(v, step float32) float32 {
	if step <= 0 {
		return v
	}
	return float32(math.Round(float64(v/step))) * step
}
