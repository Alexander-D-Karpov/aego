package scene

import "aego/core/mathx"

func (s *Scene) Local(h NodeHandle) Transform2D {
	n, ok := s.nodes.Get(h)
	if !ok {
		return IdentityTransform()
	}
	return n.local
}

func (s *Scene) local2D(h NodeHandle) (*nodeData, bool) {
	n, ok := s.node(h)
	if !ok || n.space == Space3D {
		return nil, false
	}
	return n, true
}

func (s *Scene) SetLocal(h NodeHandle, t Transform2D) {
	n, ok := s.local2D(h)
	if !ok {
		return
	}
	n.local = t
	s.markWorldDirty(h)
}

func (s *Scene) SetPosition(h NodeHandle, p mathx.Vec2) {
	n, ok := s.local2D(h)
	if !ok {
		return
	}
	n.local.Position = p
	s.markWorldDirty(h)
}

func (s *Scene) SetRotation(h NodeHandle, r float32) {
	n, ok := s.local2D(h)
	if !ok {
		return
	}
	n.local.Rotation = r
	s.markWorldDirty(h)
}

func (s *Scene) SetScale(h NodeHandle, sc mathx.Vec2) {
	n, ok := s.local2D(h)
	if !ok {
		return
	}
	n.local.Scale = sc
	s.markWorldDirty(h)
}

func (s *Scene) Local3D(h NodeHandle) Transform3D {
	n, ok := s.nodes.Get(h)
	if !ok || n.slot < 0 {
		return IdentityTransform3D()
	}
	return s.three[n.slot].local
}

func (s *Scene) local3D(h NodeHandle) (*node3, bool) {
	n, ok := s.node(h)
	if !ok || n.slot < 0 {
		return nil, false
	}
	return &s.three[n.slot], true
}

func (s *Scene) SetLocal3D(h NodeHandle, t Transform3D) {
	e, ok := s.local3D(h)
	if !ok {
		return
	}
	e.local = t
	s.markWorldDirty(h)
}

func (s *Scene) SetPosition3D(h NodeHandle, p mathx.Vec3) {
	e, ok := s.local3D(h)
	if !ok {
		return
	}
	e.local.Position = p
	s.markWorldDirty(h)
}

func (s *Scene) SetRotation3D(h NodeHandle, q mathx.Quat) {
	e, ok := s.local3D(h)
	if !ok {
		return
	}
	e.local.Rotation = q
	s.markWorldDirty(h)
}

func (s *Scene) SetScale3D(h NodeHandle, sc mathx.Vec3) {
	e, ok := s.local3D(h)
	if !ok {
		return
	}
	e.local.Scale = sc
	s.markWorldDirty(h)
}

func (s *Scene) Pose(h NodeHandle) Pose {
	n, ok := s.nodes.Get(h)
	if !ok {
		return Pose{T2: IdentityTransform(), T3: IdentityTransform3D()}
	}
	p := Pose{Space: n.space, T2: n.local, T3: IdentityTransform3D()}
	if n.slot >= 0 {
		p.T3 = s.three[n.slot].local
	}
	return p
}

func (s *Scene) SetPose(h NodeHandle, p Pose) {
	if s.Space(h) != p.Space {
		return
	}
	if p.Space == Space3D {
		s.SetLocal3D(h, p.T3)
		return
	}
	s.SetLocal(h, p.T2)
}

func (s *Scene) markWorldDirty(h NodeHandle) {
	n, ok := s.nodes.Get(h)
	if !ok || n.has(flagWorldDirty) {
		return
	}
	n.set(flagWorldDirty, true)
	for c := n.firstChild; !c.IsZero(); {
		child, ok := s.nodes.Get(c)
		if !ok {
			break
		}
		next := child.nextSib
		s.markWorldDirty(c)
		c = next
	}
}

func (s *Scene) World(h NodeHandle) mathx.Mat3 {
	n, ok := s.nodes.Get(h)
	if !ok {
		return mathx.Mat3Identity()
	}
	if n.has(flagWorldDirty) {
		s.UpdateTransforms()
		n, _ = s.nodes.Get(h)
	}
	return n.world
}

func (s *Scene) WorldPosition(h NodeHandle) mathx.Vec2 {
	m := s.World(h)
	return mathx.Vec2{X: m[6], Y: m[7]}
}

func (s *Scene) World4(h NodeHandle) mathx.Mat4 {
	n, ok := s.nodes.Get(h)
	if !ok {
		return mathx.Mat4Identity()
	}
	if n.has(flagWorldDirty) {
		s.UpdateTransforms()
		n, _ = s.nodes.Get(h)
	}
	if n.space == Space3D {
		return s.three[n.slot].world
	}
	if !n.has(flagUnder3D) {
		return mathx.Mat4Lift(n.world)
	}
	for a := n.parent; !a.IsZero(); {
		an, ok := s.nodes.Get(a)
		if !ok {
			break
		}
		if an.space == Space3D {
			return s.three[an.slot].world.Mul(mathx.Mat4Lift(n.world))
		}
		a = an.parent
	}
	return mathx.Mat4Lift(n.world)
}

func (s *Scene) WorldPosition3D(h NodeHandle) mathx.Vec3 {
	return s.World4(h).Translation()
}

func (s *Scene) Enabled(h NodeHandle) bool {
	n, ok := s.nodes.Get(h)
	return ok && n.has(flagEnabled)
}

func (s *Scene) SetEnabled(h NodeHandle, v bool) {
	n, ok := s.node(h)
	if !ok || n.has(flagEnabled) == v {
		return
	}
	n.set(flagEnabled, v)
	s.markWorldDirty(h)
	s.touch()
}

func (s *Scene) EffectiveEnabled(h NodeHandle) bool {
	n, ok := s.nodes.Get(h)
	return ok && n.has(flagEffective)
}

func (s *Scene) Layers(h NodeHandle) uint64 {
	n, ok := s.nodes.Get(h)
	if !ok {
		return 0
	}
	return n.layers
}

func (s *Scene) SetLayers(h NodeHandle, m uint64) {
	n, ok := s.node(h)
	if !ok {
		return
	}
	n.layers = m
	s.touch()
}

func (s *Scene) UpdateTransforms() {
	for _, h := range s.PreOrder() {
		n, ok := s.nodes.Get(h)
		if !ok || !n.has(flagAlive) {
			continue
		}
		if h == s.root {
			if n.has(flagWorldDirty) {
				n.world = n.local.Matrix()
				n.set(flagWorldDirty, false)
			}
			n.set(flagEffective, n.has(flagEnabled))
			continue
		}
		p, ok := s.nodes.Get(n.parent)
		if !ok {
			continue
		}
		parent3 := p.space == Space3D
		n.set(flagUnder3D, n.space == Space2D && (parent3 || p.has(flagUnder3D)))
		if n.has(flagWorldDirty) {
			switch {
			case n.space == Space3D:
				e := &s.three[n.slot]
				m := e.local.Matrix()
				if parent3 {
					m = s.three[p.slot].world.Mul(m)
				}
				e.world = m
				n.world = mathx.Mat3TRS(mathx.Vec2{X: m[12], Y: -m[13]}, 0, mathx.Vec2{X: 1, Y: 1})
			case parent3:
				n.world = n.local.Matrix()
			default:
				n.world = p.world.Mul(n.local.Matrix())
			}
			n.set(flagWorldDirty, false)
		}
		n.set(flagEffective, n.has(flagEnabled) && p.has(flagEffective) && !n.has(flagPendingDestroy))
	}
}
