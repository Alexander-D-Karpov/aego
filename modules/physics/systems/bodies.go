package systems

import (
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/modules/physics/world"
	"aego/scene"
)

func configOf(c components.Body2D) components.Body2D {
	c.Velocity, c.AngularVelocity, c.Awake = mathx.Vec2{}, 0, false
	return c
}

func bodyDef(c components.Body2D, node scene.NodeHandle) world.BodyDef {
	d := world.DefaultBodyDef()
	d.Type = world.BodyType(c.Type)
	d.MassMode = world.MassMode(c.MassMode)
	d.Mass = c.Mass
	d.GravityScale = c.GravityScale
	d.LinearDamping, d.AngularDamping = c.LinearDamping, c.AngularDamping
	d.FixedRotation, d.Bullet, d.AllowSleep = c.FixedRotation, c.Bullet, c.AllowSleep
	d.Layer = uint8(min(max(c.Layer, 0), 31))
	d.Mask = uint32(c.Mask)
	d.UserData = Pack(node)
	return d
}

func (m *Sim) velocity(b *bodyRec) (mathx.Vec2, float32) {
	if b.h.IsZero() {
		return mathx.Vec2{}, 0
	}
	v, av, _ := m.w.BodyVelocity(b.h)
	return v, av
}

func (m *Sim) createBody(b *bodyRec, v mathx.Vec2, av float32) {
	s := m.s
	if s.Under3D(b.node) {
		m.report(b.node, keySpace, CodeSpace, "physics needs a 2D node that is not under a 3D parent", true)
		return
	}
	m.resolve(b.node, keySpace)
	def := world.DefaultBodyDef()
	if b.implicit {
		def.Type = world.Static
		def.UserData = Pack(b.node)
		b.cfg = components.Body2D{}
	} else {
		c, ok := scene.Get[components.Body2D](s, b.node)
		if !ok {
			return
		}
		b.cfg = configOf(*c)
		def = bodyDef(b.cfg, b.node)
		def.LinearVelocity, def.AngularVelocity = v, av
	}
	wm := s.World(b.node)
	f := frame(wm)
	def.Position, def.Angle = f.p, f.a
	h, err := m.w.CreateBody(def)
	if err != nil {
		m.report(b.node, keyBody, CodeBody, "Body2D: "+err.Error(), true)
		return
	}
	m.resolve(b.node, keyBody)
	b.h = h
	b.local, b.world = s.Local(b.node), wm
	b.prev, b.cur = f, f
}

func (m *Sim) dropBody(b *bodyRec) {
	if !b.h.IsZero() {
		m.w.DestroyBody(b.h)
	}
	b.h = world.BodyHandle{}
	b.excluded = false
}

func (m *Sim) syncBody(b *bodyRec) {
	s := m.s
	if !b.implicit {
		c, ok := scene.Get[components.Body2D](s, b.node)
		if !ok {
			m.dirty = true
			return
		}
		if cfg := configOf(*c); cfg != b.cfg {
			switch {
			case b.h.IsZero() || cfg.Type != b.cfg.Type:
				v, av := m.velocity(b)
				m.dropBody(b)
				m.createBody(b, v, av)
			default:
				b.cfg = cfg
				if err := m.w.SetBodyProps(b.h, bodyDef(cfg, b.node)); err != nil {
					m.report(b.node, keyBody, CodeBody, "Body2D: "+err.Error(), true)
				} else {
					m.resolve(b.node, keyBody)
				}
			}
		}
	}
	if b.h.IsZero() {
		return
	}
	if b.moves() {
		if s.Local(b.node) != b.local {
			m.teleport(b)
		}
		return
	}
	if s.World(b.node) != b.world {
		m.teleport(b)
	}
}

func (m *Sim) teleport(b *bodyRec) {
	s := m.s
	wm := s.World(b.node)
	f := frame(wm)
	if err := m.w.SetBodyTransform(b.h, f.p, f.a); err != nil {
		m.report(b.node, keyBody, CodeBody, "Body2D: "+err.Error(), true)
		return
	}
	b.local, b.world = s.Local(b.node), wm
	b.prev, b.cur = f, f
}

func (m *Sim) reparent(b *bodyRec) {
	v, av := m.velocity(b)
	p, a, ok := m.w.BodyTransform(b.h)
	m.dropBody(b)
	if ok {
		m.s.SetLocal(b.node, m.toLocal(b.node, pose{p: p, a: a}))
	}
	m.report(b.node, keyReparent, CodeReparent, "Body2D was reparented during Play; the body was recreated with its world pose and velocity", false)
	m.createBody(b, v, av)
}

func (m *Sim) toLocal(h scene.NodeHandle, f pose) scene.Transform2D {
	s := m.s
	l := s.Local(h)
	pw := s.World(s.Parent(h))
	inv, ok := pw.Inverse()
	if !ok {
		return l
	}
	l.Position = inv.Apply(f.p)
	l.Rotation = f.a - mathx.Atan2(pw[1], pw[0])
	return l
}

func (m *Sim) Writeback() {
	s := m.s
	for _, b := range m.bodies {
		if !b.moves() || b.h.IsZero() {
			continue
		}
		v, av, _ := m.w.BodyVelocity(b.h)
		awake := m.w.BodyAwake(b.h)
		if c, ok := scene.Get[components.Body2D](s, b.node); ok {
			c.Velocity, c.AngularVelocity, c.Awake = v, av, awake
		}
		p, a, _ := m.w.BodyTransform(b.h)
		b.prev, b.cur = b.cur, pose{p: p, a: a}
		if b.prev == b.cur {
			continue
		}
		l := m.toLocal(b.node, b.cur)
		s.SetLocal(b.node, l)
		b.local = l
	}
}

func (m *Sim) Present(alpha float32) {
	s := m.s
	s.ClearPresentation()
	for _, b := range m.bodies {
		if !b.moves() || b.h.IsZero() {
			continue
		}
		if s.Local(b.node) != b.local {
			if !b.warned {
				b.warned = true
				m.report(b.node, keyUpdate, CodeUpdate, "Transform2D of a moving physics body was changed outside FixedUpdate; it is applied as a teleport at the next physics step", false)
			}
			continue
		}
		if !b.cfg.Interpolate || b.prev == b.cur {
			continue
		}
		s.SetPresentationLocal(b.node, m.toLocal(b.node, lerpPose(b.prev, b.cur, alpha)))
	}
}

func (m *Sim) Teleport(h scene.NodeHandle, p mathx.Vec2, a float32) error {
	b := m.byBody[h]
	if b == nil || b.h.IsZero() {
		return errNoBody
	}
	if err := m.w.SetBodyTransform(b.h, p, a); err != nil {
		return err
	}
	f := pose{p: p, a: a}
	l := m.toLocal(h, f)
	m.s.SetLocal(h, l)
	b.local, b.world = l, m.s.World(h)
	b.prev, b.cur = f, f
	return nil
}
