package scene

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/core/mathx"
	"aego/schema"
)

type Space uint8

const (
	Space2D Space = iota
	Space3D
)

func (sp Space) String() string {
	if sp == Space3D {
		return "3d"
	}
	return "2d"
}

func (sp Space) Kind() schema.SpaceKind {
	if sp == Space3D {
		return schema.Space3D
	}
	return schema.Space2D
}

func ParseSpace(s string) (Space, bool) {
	switch s {
	case "2d":
		return Space2D, true
	case "3d":
		return Space3D, true
	}
	return Space2D, false
}

type Transform3D struct {
	Position mathx.Vec3
	Rotation mathx.Quat
	Scale    mathx.Vec3
}

func IdentityTransform3D() Transform3D {
	return Transform3D{Rotation: mathx.QuatIdentity(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}
}

func (t Transform3D) Matrix() mathx.Mat4 {
	rot := t.Rotation
	if rot.IsZero() {
		rot = mathx.QuatIdentity()
	}
	sc := t.Scale
	if sc == (mathx.Vec3{}) {
		sc = mathx.Vec3{X: 1, Y: 1, Z: 1}
	}
	return mathx.Mat4TRS(t.Position, rot, sc)
}

func Lift(t Transform2D) Transform3D {
	sc := t.Scale
	if sc.X == 0 && sc.Y == 0 {
		sc = mathx.Vec2{X: 1, Y: 1}
	}
	return Transform3D{
		Position: mathx.Vec3{X: t.Position.X, Y: -t.Position.Y},
		Rotation: mathx.QuatAxisAngle(mathx.Vec3{Z: 1}, -t.Rotation),
		Scale:    mathx.Vec3{X: sc.X, Y: sc.Y, Z: 1},
	}
}

func Flatten(t Transform3D) Transform2D {
	_, _, z := t.Rotation.Euler()
	return Transform2D{
		Position: mathx.Vec2{X: t.Position.X, Y: -t.Position.Y},
		Rotation: -z,
		Scale:    mathx.Vec2{X: t.Scale.X, Y: t.Scale.Y},
	}
}

type Pose struct {
	Space Space
	T2    Transform2D
	T3    Transform3D
}

type node3 struct {
	owner NodeHandle
	local Transform3D
	world mathx.Mat4
}

func (s *Scene) alloc3(h NodeHandle, t Transform3D) int32 {
	e := node3{owner: h, local: t, world: mathx.Mat4Identity()}
	if n := len(s.free3); n > 0 {
		slot := s.free3[n-1]
		s.free3 = s.free3[:n-1]
		s.three[slot] = e
		return slot
	}
	s.three = append(s.three, e)
	return int32(len(s.three) - 1)
}

func (s *Scene) release3(slot int32) {
	s.three[slot] = node3{}
	s.free3 = append(s.free3, slot)
}

func (s *Scene) DefaultSpace() Space {
	return s.defaultSpace
}

func (s *Scene) SetDefaultSpace(sp Space) {
	if s.defaultSpace != sp {
		s.defaultSpace = sp
		s.touch()
	}
}

func (s *Scene) Space(h NodeHandle) Space {
	n, ok := s.nodes.Get(h)
	if !ok {
		return Space2D
	}
	return n.space
}

func (s *Scene) Under3D(h NodeHandle) bool {
	n, ok := s.nodes.Get(h)
	if !ok || n.space == Space3D {
		return false
	}
	if n.has(flagWorldDirty) {
		s.UpdateTransforms()
		n, _ = s.nodes.Get(h)
	}
	return n.has(flagUnder3D)
}

func (s *Scene) AllowsSpace(parent NodeHandle, sp Space) bool {
	if sp == Space2D || parent.IsZero() || parent == s.root {
		return true
	}
	p, ok := s.nodes.Get(parent)
	return ok && p.space == Space3D
}

func (s *Scene) spaceError(node string, parent NodeHandle) error {
	return errs.New(errs.SceneSpaceMismatch, "a 3D node needs a 3D parent or the scene root",
		log.F("node", node), log.F("parent", s.Name(parent)))
}

func (s *Scene) Accepts(h NodeHandle, t schema.TypeID) bool {
	td, ok := s.registry.ByID(t)
	return ok && td.Space.Allows(s.Space(h).Kind())
}

func (s *Scene) SetSpace(h NodeHandle, sp Space) error {
	n, ok := s.node(h)
	if !ok || n.has(flagPendingDestroy) {
		return errs.New(errs.SceneDeadNode, "node is not alive")
	}
	if h == s.root {
		return errs.New(errs.SceneSpaceMismatch, "the scene root has no space of its own")
	}
	if n.space == sp {
		return nil
	}
	if n.childCount > 0 {
		return errs.New(errs.SceneSpaceMismatch, "a node with children cannot change its space", log.F("node", s.Name(h)))
	}
	if !s.AllowsSpace(n.parent, sp) {
		return s.spaceError(s.Name(h), n.parent)
	}
	for _, st := range s.storeList {
		if st.hasNode(h) && !st.descriptor().Space.Allows(sp.Kind()) {
			return errs.New(errs.SceneSpaceMismatch, "a component on the node is tied to its current space",
				log.F("node", s.Name(h)), log.F("component", st.descriptor().FullName))
		}
	}
	if sp == Space3D {
		n.slot = s.alloc3(h, Lift(n.local))
		n.local = IdentityTransform()
	} else {
		n.local = Flatten(s.three[n.slot].local)
		s.release3(n.slot)
		n.slot = -1
	}
	n.space = sp
	n.set(flagWorldDirty, false)
	s.markWorldDirty(h)
	s.touch()
	s.emit(Event{Kind: EvSpaceChanged, Node: h, ID: n.id})
	return nil
}
