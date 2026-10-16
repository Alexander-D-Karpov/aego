package systems

import (
	"aego/core/mathx"
	"aego/modules/physics/components"
	"aego/scene"
)

func OwnerOf(s *scene.Scene, h scene.NodeHandle) scene.NodeHandle {
	root := s.Root()
	for x := h; !x.IsZero() && x != root; x = s.Parent(x) {
		if scene.Has[components.Body2D](s, x) {
			return x
		}
	}
	return h
}

func Relative(s *scene.Scene, h scene.NodeHandle) mathx.Mat3 {
	return relative(s.World(OwnerOf(s, h)), s.World(h))
}
