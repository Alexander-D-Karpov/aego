package editor

import (
	"aego/contracts/debugdraw"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/shape"
	"aego/editor/core/vectors"
	"aego/engine"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/modules/physics/systems"
	"aego/scene"
)

var (
	colStatic    = mathx.Color{R: 0.5, G: 0.85, B: 0.5, A: 1}
	colKinematic = mathx.Color{R: 0.5, G: 0.6, B: 0.95, A: 1}
	colDynamic   = mathx.Color{R: 0.95, G: 0.7, B: 0.55, A: 1}
	colTrigger   = mathx.Color{R: 1, G: 0.9, B: 0.3, A: 0.8}
	colOff       = mathx.Color{R: 0.55, G: 0.55, B: 0.55, A: 0.6}
	colJoint     = mathx.Color{R: 0.5, G: 0.8, B: 0.8, A: 1}
)

var channelFlags = [...]struct{ flag, channel string }{
	{FlagShapes, physics.ChannelShapes},
	{FlagContacts, physics.ChannelContacts},
	{FlagAABBs, physics.ChannelAABBs},
	{FlagJoints, physics.ChannelJoints},
	{FlagIslands, physics.ChannelIslands},
	{FlagBroad, physics.ChannelBroadphase},
}

// syncChannels applies the debug layers to the play world every frame, so turning the Game View overlays off hides them even though no overlay draws.
func (p *plugin) syncChannels() {
	if w := p.ed.PlayWorld(); w != nil {
		p.applyChannels(w)
	}
}

// applyChannels shows a physics channel only while the Game View overlays, the debug overlay and the channel's layer are all on; the layer flags themselves are kept.
func (p *plugin) applyChannels(w *engine.World) {
	d, ok := engine.ServiceOf[debugdraw.Draw](w, debugdraw.ServiceID)
	if !ok {
		return
	}
	o := &p.ed.Overlay
	master := o.On(context.OverlayGame) && o.On(CmdDebug)
	for _, cf := range channelFlags {
		d.Hide(cf.channel, !(master && o.On(cf.flag)))
	}
}

func (p *plugin) overlay(c *shape.OverlayCtx) {
	if c.Mode3D || p.ed == nil {
		return
	}
	o := &p.ed.Overlay
	sim := physics.SimOf(c.World)
	if sim != nil && sim.Scene() == c.Scene {
		want := o.On(vectors.CommandToggle) && o.On(vectors.Prefix+layerForces)
		if sim.World().Config().DebugVectors != want {
			sim.World().SetDebugVectors(want)
		}
		p.applyChannels(c.World)
		return
	}
	if !o.On(CmdDebug) || c.Game {
		return
	}
	p.drawScene(c)
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
