package editor

import (
	"aego/core/mathx"
	"aego/editor/core/shape"
	"aego/editor/core/vectors"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/physics/systems"
	"aego/modules/physics/world"
	"aego/render/debug"
	"aego/scene"
)

var (
	colStatic    = mathx.Color{R: 0.5, G: 0.85, B: 0.5, A: 1}
	colKinematic = mathx.Color{R: 0.5, G: 0.6, B: 0.95, A: 1}
	colDynamic   = mathx.Color{R: 0.95, G: 0.7, B: 0.55, A: 1}
	colTrigger   = mathx.Color{R: 1, G: 0.9, B: 0.3, A: 0.8}
	colOff       = mathx.Color{R: 0.55, G: 0.55, B: 0.55, A: 0.6}
	colJoint     = mathx.Color{R: 0.5, G: 0.8, B: 0.8, A: 1}
	treeColors   = [...]mathx.Color{
		{R: 0.95, G: 0.45, B: 0.35, A: 0.5}, {R: 0.35, G: 0.75, B: 0.95, A: 0.5}, {R: 0.55, G: 0.9, B: 0.4, A: 0.5},
		{R: 0.95, G: 0.8, B: 0.3, A: 0.5}, {R: 0.8, G: 0.45, B: 0.95, A: 0.5}, {R: 0.3, G: 0.9, B: 0.8, A: 0.5},
	}
)

type drawer struct {
	d *debug.Draw
}

func (r drawer) Line(a, b mathx.Vec2, c mathx.Color) {
	r.d.Line(a, b, c)
}

func (r drawer) Circle(center mathx.Vec2, radius float32, segments int, c mathx.Color) {
	r.d.Circle(center, radius, segments, c)
}

func (p *plugin) overlay(c *shape.OverlayCtx) {
	if c.Mode3D || p.ed == nil {
		return
	}
	o := &p.ed.Overlay
	sim := physics.SimOf(c.World)
	live := sim != nil && sim.Scene() == c.Scene
	if live {
		want := o.On(vectors.CommandToggle) && o.On(vectors.Prefix+layerForces)
		if sim.World().Config().DebugVectors != want {
			sim.World().SetDebugVectors(want)
		}
	}
	if !o.On(CmdDebug) {
		return
	}
	if live {
		p.drawWorld(c, sim.World())
		return
	}
	if c.Game {
		return
	}
	p.drawScene(c)
}

func (p *plugin) drawWorld(c *shape.OverlayCtx, w *world.World) {
	o := &p.ed.Overlay
	var f world.DebugFlags
	for _, x := range [...]struct {
		id   string
		flag world.DebugFlags
	}{
		{FlagShapes, world.DrawShapes},
		{FlagContacts, world.DrawContacts},
		{FlagAABBs, world.DrawAABBs},
		{FlagJoints, world.DrawJoints},
		{FlagIslands, world.DrawIslands | world.DrawShapes},
	} {
		if o.On(x.id) {
			f |= x.flag
		}
	}
	w.DebugDraw(drawer{c.Debug}, f)
	if !o.On(FlagBroad) {
		return
	}
	w.DebugTree(func(b world.AABB, height int32, leaf bool) {
		if leaf {
			return
		}
		c.Debug.AABB(b, treeColors[int(height)%len(treeColors)])
	})
}

func (p *plugin) colorOf(s *scene.Scene, h scene.NodeHandle, k components.Kind) mathx.Color {
	if _, bad := p.issue(s.ID(h), components.Names[k]); bad {
		return colBad
	}
	if !s.EffectiveEnabled(h) {
		return colOff
	}
	if p.col.Common.IsTrigger {
		return colTrigger
	}
	if b, ok := scene.Get[components.Body2D](s, systems.OwnerOf(s, h)); ok {
		switch b.Type {
		case components.Dynamic:
			return colDynamic
		case components.Kinematic:
			return colKinematic
		}
	}
	return colStatic
}

func (p *plugin) drawScene(c *shape.OverlayCtx) {
	o := &p.ed.Overlay
	s := c.Scene
	if s == nil {
		return
	}
	s.UpdateTransforms()
	root := s.Root()
	for _, h := range s.PreOrder() {
		if h == root || !s.IsAlive(h) || s.Under3D(h) {
			continue
		}
		if o.On(FlagShapes) {
			for k := components.Kind(0); k < components.KindCount; k++ {
				if !components.Read(s, h, k, &p.col) {
					continue
				}
				var closed bool
				p.pts, closed = outline(&p.col, p.pts)
				drawPoly(c.Debug, s.World(h), p.pts, closed, p.colorOf(s, h, k))
			}
		}
		if o.On(FlagJoints) {
			p.drawJoints(c, s, h)
		}
	}
}

func (p *plugin) drawJoints(c *shape.OverlayCtx, s *scene.Scene, h scene.NodeHandle) {
	m := s.World(h)
	if j, ok := scene.Get[components.DistanceJoint2D](s, h); ok {
		a := m.Apply(j.AnchorA)
		if th := s.Handle(j.Target.NodeRefID()); !th.IsZero() && s.IsAlive(th) {
			c.Debug.Line(a, s.World(th).Apply(j.AnchorB), colJoint)
		}
	}
	if j, ok := scene.Get[components.RevoluteJoint2D](s, h); ok {
		a := m.Apply(j.Anchor)
		c.Debug.Circle(a, c.Px*6, 12, colJoint)
		if th := s.Handle(j.Target.NodeRefID()); !th.IsZero() && s.IsAlive(th) {
			c.Debug.Line(a, s.WorldPosition(th), colJoint)
		}
	}
}
