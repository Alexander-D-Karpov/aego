package props

import (
	"aego/core/ids"
	"aego/scene"
	"aego/schema"
)

func (a *Access) SpaceLock(node ids.NodeID) (bool, string) {
	s := a.s
	h := s.Handle(node)
	if h.IsZero() || !s.IsAlive(h) {
		return true, "node is not in the scene"
	}
	if h == s.Root() {
		return true, "the scene root has no space of its own"
	}
	children := false
	s.Children(h, func(scene.NodeHandle) bool {
		children = true
		return false
	})
	if children {
		return true, "a node with children keeps its space"
	}
	other := scene.Space3D
	if s.Space(h) == scene.Space3D {
		other = scene.Space2D
	}
	if !s.AllowsSpace(s.Parent(h), other) {
		return true, "a 3D node needs a 3D parent or the scene root"
	}
	reason := ""
	s.ComponentsOf(h, func(id schema.TypeID) {
		if reason != "" {
			return
		}
		if td, ok := s.Registry().ByID(id); ok && !td.Space.Allows(other.Kind()) {
			reason = td.FullName + " is tied to the current space"
		}
	})
	return reason != "", reason
}

func (a *Access) Fits(nodes []ids.NodeID, t schema.TypeID) bool {
	if len(nodes) == 0 {
		return false
	}
	for _, n := range nodes {
		h := a.s.Handle(n)
		if h.IsZero() || !a.s.Accepts(h, t) {
			return false
		}
	}
	return true
}
