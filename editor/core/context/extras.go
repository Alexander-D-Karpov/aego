package context

import (
	"aego/core/mathx"
	"aego/engine"
	"aego/render/debug"
	"aego/render/view"
	"aego/scene"
)

const DefaultVectorScale = 0.1

type SettingsPage struct {
	Section string
	Title   string
	Order   int
	Draw    func(draft any) bool
}

type VectorLayer struct {
	ID    string
	Title string
	Draw  func(*VectorSink)
}

type ProfilerSection struct {
	Title string
	Play  bool
	Draw  func()
}

type Overlays struct {
	on    map[string]bool
	Scale float32
	Fixed bool
}

func (o *Overlays) On(id string) bool {
	return o.on[id]
}

func (o *Overlays) Set(id string, v bool) {
	if o.on == nil {
		o.on = map[string]bool{}
	}
	o.on[id] = v
}

func (o *Overlays) Toggle(id string) {
	o.Set(id, !o.On(id))
}

func (o *Overlays) VectorScale() float32 {
	if o.Scale <= 0 {
		return DefaultVectorScale
	}
	return o.Scale
}

type VectorSink struct {
	Editor *Editor
	Scene  *scene.Scene
	World  *engine.World
	Debug  *debug.Draw
	View   view.View
	Px     float32
	Scale  float32
	Fixed  bool
}

func (v *VectorSink) length(vec mathx.Vec2) mathx.Vec2 {
	if !v.Fixed {
		return vec.Scale(v.Scale)
	}
	l := vec.Len()
	if l == 0 {
		return vec
	}
	return vec.Scale(v.Px * 48 / l)
}

func (v *VectorSink) head(tip, dir mathx.Vec2, size float32, col mathx.Color) {
	v.Debug.Line(tip, tip.Sub(dir.Rotated(0.45).Scale(size)), col)
	v.Debug.Line(tip, tip.Sub(dir.Rotated(-0.45).Scale(size)), col)
}

func (v *VectorSink) Arrow(from, vec mathx.Vec2, col mathx.Color) {
	d := v.length(vec)
	l := d.Len()
	if l < v.Px {
		return
	}
	to := from.Add(d)
	v.Debug.Line(from, to, col)
	v.head(to, d.Scale(1/l), min(v.Px*10, l*0.5), col)
}

func (v *VectorSink) Arc(center mathx.Vec2, omega float32, col mathx.Color) {
	if omega == 0 {
		return
	}
	r := v.Px * 18
	sweep := mathx.Pi * 1.5
	if !v.Fixed {
		sweep = mathx.Clamp(mathx.Abs(omega)*v.Scale*mathx.TwoPi, 0.2, mathx.TwoPi*0.9)
	}
	dir := float32(1)
	if omega < 0 {
		dir = -1
	}
	const segs = 24
	prev := center.Add(mathx.Vec2{X: r})
	var a float32
	for i := 1; i <= segs; i++ {
		a = dir * sweep * float32(i) / segs
		p := center.Add(mathx.Vec2{X: r * mathx.Cos(a), Y: r * mathx.Sin(a)})
		v.Debug.Line(prev, p, col)
		prev = p
	}
	t := mathx.Vec2{X: -mathx.Sin(a) * dir, Y: mathx.Cos(a) * dir}
	v.head(prev, t, v.Px*8, col)
}

func (v *VectorSink) Circle(center mathx.Vec2, r float32, col mathx.Color) {
	v.Debug.Circle(center, r, 32, col)
}
