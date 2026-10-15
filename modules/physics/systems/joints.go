package systems

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/modules/physics/world"
	"aego/scene"
)

func (m *Sim) bodyAt(h scene.NodeHandle) *bodyRec {
	root := m.s.Root()
	for x := h; !x.IsZero() && x != root; x = m.s.Parent(x) {
		if b := m.byBody[x]; b != nil {
			return b
		}
	}
	return nil
}

func (m *Sim) jointAlive(j *jointRec) bool {
	if j.h.IsZero() {
		return false
	}
	_, ok := m.w.JointKindOf(j.h)
	return ok
}

func (m *Sim) dropJoint(j *jointRec) {
	if m.jointAlive(j) {
		m.w.DestroyJoint(j.h)
	}
	j.h = world.JointHandle{}
}

func (m *Sim) local(b *bodyRec, wp mathx.Vec2) mathx.Vec2 {
	p, a, _ := m.w.BodyTransform(b.h)
	return toBody(pose{p: p, a: a}, wp)
}

func (m *Sim) anchor(b *bodyRec, node scene.NodeHandle, at mathx.Vec2) mathx.Vec2 {
	return m.local(b, m.s.World(node).Apply(at))
}

func (m *Sim) jointDef(j *jointRec, ba, bb *bodyRec, target scene.NodeHandle) world.JointDef {
	node := j.key.node
	if j.key.kind == jointDistance {
		c := &j.dist
		return world.DistanceJoint{
			A: ba.h, B: bb.h,
			LocalAnchorA:     m.anchor(ba, node, c.AnchorA),
			LocalAnchorB:     m.anchor(bb, target, c.AnchorB),
			Length:           c.Length,
			Min:              c.MinLength,
			Max:              c.MaxLength,
			Stiffness:        c.Stiffness,
			Damping:          c.Damping,
			CollideConnected: c.CollideConnected,
		}
	}
	c := &j.rev
	wp := m.s.World(node).Apply(c.Anchor)
	return world.RevoluteJoint{
		A: ba.h, B: bb.h,
		LocalAnchorA:     m.local(ba, wp),
		LocalAnchorB:     m.local(bb, wp),
		EnableLimit:      c.EnableLimit,
		Lower:            c.Lower,
		Upper:            c.Upper,
		EnableMotor:      c.EnableMotor,
		Speed:            c.Speed,
		MaxTorque:        c.MaxTorque,
		CollideConnected: c.CollideConnected,
	}
}

func (m *Sim) syncJoint(j *jointRec) {
	s := m.s
	node := j.key.node
	same := j.built
	var target ids.NodeID
	if j.key.kind == jointDistance {
		c, ok := scene.Get[components.DistanceJoint2D](s, node)
		if !ok {
			m.dirty = true
			return
		}
		same = same && *c == j.dist
		j.dist = *c
		target = j.dist.Target.NodeRefID()
	} else {
		c, ok := scene.Get[components.RevoluteJoint2D](s, node)
		if !ok {
			m.dirty = true
			return
		}
		same = same && *c == j.rev
		j.rev = *c
		target = j.rev.Target.NodeRefID()
	}
	key := jointKeyName(j.key.kind)
	th := s.Handle(target)
	ba := m.bodyAt(node)
	var bb *bodyRec
	if !th.IsZero() && s.IsAlive(th) {
		bb = m.bodyAt(th)
	}
	if ba == nil || bb == nil || ba == bb || ba.h.IsZero() || bb.h.IsZero() {
		m.dropJoint(j)
		j.built = false
		m.report(node, key, CodeJoint, "joint needs a physics body on its node and a different body on its Target", true)
		return
	}
	if same && j.a == ba.h && j.b == bb.h && (j.failed || m.jointAlive(j)) {
		return
	}
	m.dropJoint(j)
	j.built, j.a, j.b = true, ba.h, bb.h
	h, err := m.w.CreateJoint(m.jointDef(j, ba, bb, th))
	j.failed = err != nil
	if err != nil {
		m.report(node, key, CodeJoint, err.Error(), true)
		return
	}
	j.h = h
	m.resolve(node, key)
}
