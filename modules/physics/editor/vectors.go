package editor

import (
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/engine/module"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/physics/settings"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/scene"
)

var (
	colVelocity = mathx.Color{R: 0.3, G: 1, B: 0.4, A: 1}
	colAngular  = mathx.Color{R: 0.3, G: 0.9, B: 0.9, A: 1}
	colForce    = mathx.Color{R: 1, G: 0.6, B: 0.2, A: 1}
	colImpulse  = mathx.Color{R: 1, G: 0.3, B: 0.3, A: 1}
	colGravity  = mathx.Color{R: 0.7, G: 0.7, B: 0.7, A: 1}
)

func simOf(v *context.VectorSink) *systems.Sim {
	if v.World == nil {
		return nil
	}
	s := physics.SimOf(v.World)
	if s == nil || s.Scene() != v.Scene {
		return nil
	}
	return s
}

func (p *plugin) velocity(v *context.VectorSink) {
	sim := simOf(v)
	if sim == nil {
		return
	}
	w := sim.World()
	w.ForEachBody(func(h world.BodyHandle) bool {
		if t, _ := w.BodyType(h); t == world.Static {
			return true
		}
		c, _ := w.BodyCenter(h)
		vel, _, _ := w.BodyVelocity(h)
		v.Arrow(c, vel, colVelocity)
		return true
	})
}

func (p *plugin) angular(v *context.VectorSink) {
	sim := simOf(v)
	if sim == nil {
		return
	}
	w := sim.World()
	w.ForEachBody(func(h world.BodyHandle) bool {
		if t, _ := w.BodyType(h); t == world.Static {
			return true
		}
		c, _ := w.BodyCenter(h)
		_, av, _ := w.BodyVelocity(h)
		v.Arc(c, av, colAngular)
		return true
	})
}

func (p *plugin) forces(v *context.VectorSink) {
	sim := simOf(v)
	if sim == nil {
		return
	}
	for _, f := range sim.World().Forces() {
		v.Arrow(f.Point, f.Vector, colForce)
	}
}

func (p *plugin) contacts(v *context.VectorSink) {
	sim := simOf(v)
	if sim == nil {
		return
	}
	w := sim.World()
	w.ForEachBody(func(h world.BodyHandle) bool {
		w.Contacts(h, func(c world.Contact) bool {
			if c.BodyA != h || c.Trigger {
				return true
			}
			m := &c.Manifold
			for i := 0; i < m.Count; i++ {
				pt := &m.Points[i]
				v.Arrow(pt.Point, m.Normal.Scale(pt.TotalNormalImpulse), colImpulse)
			}
			return true
		})
		return true
	})
}

func (p *plugin) gravity(v *context.VectorSink) {
	var g mathx.Vec2
	switch sim := simOf(v); {
	case sim != nil:
		g = sim.World().Gravity()
	default:
		g = world.DefaultConfig().Gravity
		if p.ed != nil {
			if s, ok := module.SectionValue[settings.Project](p.ed.Sections, settings.ID); ok {
				g = s.Config().Gravity
			}
		}
	}
	b := v.View.VisibleBounds()
	v.Arrow(mathx.Vec2{X: b.Min.X + v.Px*60, Y: b.Min.Y + v.Px*60}, g, colGravity)
	if v.Scene == nil {
		return
	}
	scene.Query1[components.Body2D](v.Scene).Each(func(h scene.NodeHandle, body *components.Body2D) {
		if body.Type == components.Dynamic && body.GravityScale != 1 {
			v.Arrow(v.Scene.WorldPosition(h), g.Scale(body.GravityScale), colGravity)
		}
	})
}
