package scene

import (
	"aego/core/mathx"
)

func (s *Scene) Local(h NodeHandle) Transform2D {
	n, ok := s.nodes.Get(h)
	if !ok {
		return IdentityTransform()
	}
	return n.local
}

func (s *Scene) SetLocal(h NodeHandle, t Transform2D) {
	n, ok := s.node(h)
	if !ok {
		return
	}
	n.local = t
	s.markWorldDirty(h)
}

func (s *Scene) SetPosition(h NodeHandle, p mathx.Vec2) {
	n, ok := s.node(h)
	if !ok {
		return
	}
	n.local.Position = p
	s.markWorldDirty(h)
}

func (s *Scene) SetRotation(h NodeHandle, r float32) {
	n, ok := s.node(h)
	if !ok {
		return
	}
	n.local.Rotation = r
	s.markWorldDirty(h)
}

func (s *Scene) SetScale(h NodeHandle, sc mathx.Vec2) {
	n, ok := s.node(h)
	if !ok {
		return
	}
	n.local.Scale = sc
	s.markWorldDirty(h)
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
	order := s.PreOrder()
	for _, h := range order {
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
		if n.has(flagWorldDirty) {
			n.world = p.world.Mul(n.local.Matrix())
			n.set(flagWorldDirty, false)
		}
		n.set(flagEffective, n.has(flagEnabled) && p.has(flagEffective) && !n.has(flagPendingDestroy))
	}
}
