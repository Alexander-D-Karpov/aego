package systems

import (
	"errors"

	"aego/contracts/physics2d"
	"aego/core/mathx"
	"aego/modules/physics/world"
	"aego/scene"
)

var errNoBody = errors.New("node has no simulated physics body")

func (m *Sim) castHit(h world.CastHit) physics2d.CastHit {
	return physics2d.CastHit{
		Point:        h.Point,
		Normal:       h.Normal,
		Fraction:     h.Fraction,
		Depth:        h.Depth,
		Node:         m.BodyNode(h.Body),
		ColliderNode: m.ShapeNode(h.Shape),
		Initial:      h.Initial,
	}
}

func (m *Sim) simBody(h scene.NodeHandle) (world.BodyHandle, bool) {
	b := m.byBody[h]
	if b == nil || b.h.IsZero() {
		return world.BodyHandle{}, false
	}
	return b.h, true
}

func (m *Sim) CastCircle(c mathx.Vec2, r float32, delta mathx.Vec2, f physics2d.QueryFilter) (physics2d.CastHit, bool) {
	h, ok := m.w.CastCircle(c, r, delta, m.filter(f))
	if !ok {
		return physics2d.CastHit{}, false
	}
	return m.castHit(h), true
}

func (m *Sim) CastBox(c, half mathx.Vec2, angle float32, delta mathx.Vec2, f physics2d.QueryFilter) (physics2d.CastHit, bool) {
	h, ok := m.w.CastBox(c, half, angle, delta, m.filter(f))
	if !ok {
		return physics2d.CastHit{}, false
	}
	return m.castHit(h), true
}

func (m *Sim) CastCapsule(a, b mathx.Vec2, r float32, delta mathx.Vec2, f physics2d.QueryFilter) (physics2d.CastHit, bool) {
	h, ok := m.w.CastCapsule(a, b, r, delta, m.filter(f))
	if !ok {
		return physics2d.CastHit{}, false
	}
	return m.castHit(h), true
}

func (m *Sim) CastBody(body scene.NodeHandle, offset, delta mathx.Vec2, f physics2d.QueryFilter) (physics2d.CastHit, bool) {
	bh, ok := m.simBody(body)
	if !ok {
		return physics2d.CastHit{}, false
	}
	h, ok := m.w.CastBody(bh, offset, delta, m.filter(f))
	if !ok {
		return physics2d.CastHit{}, false
	}
	return m.castHit(h), true
}

func (m *Sim) Penetration(body scene.NodeHandle, f physics2d.QueryFilter, out []physics2d.CastHit) int {
	bh, ok := m.simBody(body)
	if !ok || len(out) == 0 {
		return 0
	}
	if cap(m.qcasts) < len(out) {
		m.qcasts = make([]world.CastHit, len(out))
	}
	buf := m.qcasts[:len(out)]
	n := m.w.Penetration(bh, m.filter(f), buf)
	for i := 0; i < n; i++ {
		out[i] = m.castHit(buf[i])
	}
	return n
}

func (m *Sim) BodyPose(body scene.NodeHandle) (mathx.Vec2, float32, bool) {
	bh, ok := m.simBody(body)
	if !ok {
		return mathx.Vec2{}, 0, false
	}
	return m.w.BodyTransform(bh)
}

func (m *Sim) BodyVelocity(body scene.NodeHandle) (mathx.Vec2, float32, bool) {
	bh, ok := m.simBody(body)
	if !ok {
		return mathx.Vec2{}, 0, false
	}
	return m.w.BodyVelocity(bh)
}

func (m *Sim) MoveKinematic(body scene.NodeHandle, p mathx.Vec2, angle, dt float32) error {
	bh, ok := m.simBody(body)
	if !ok {
		return errNoBody
	}
	return m.w.MoveKinematic(bh, world.Transform{Position: p, Angle: angle}, dt)
}

func (m *Sim) ApplyImpulse(body scene.NodeHandle, j mathx.Vec2) error {
	bh, ok := m.simBody(body)
	if !ok {
		return errNoBody
	}
	return m.w.ApplyImpulse(bh, j)
}

func (m *Sim) Gravity() mathx.Vec2 {
	return m.w.Gravity()
}
