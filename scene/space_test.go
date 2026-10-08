package scene

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/schema"
)

func spaceScene(t *testing.T) *Scene {
	t.Helper()
	types := NewTypes(schema.NewRegistry())
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return New(types, nil, Options{})
}

func createIn(t *testing.T, s *Scene, parent NodeHandle, name string, sp Space) NodeHandle {
	t.Helper()
	h, err := s.CreateIn(parent, name, ids.NodeID{}, sp)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestSpaceInheritance(t *testing.T) {
	s := spaceScene(t)
	a := s.Create3D(s.Root(), "a")
	if b := s.Create(a, "b"); s.Space(b) != Space3D {
		t.Fatal("child of a 3D node must inherit 3D")
	}
	c := createIn(t, s, a, "c", Space2D)
	if s.Space(c) != Space2D || !s.Under3D(c) {
		t.Fatal("2D child under 3D parent must be allowed and marked Under3D")
	}
	if flat := s.Create(s.Root(), "flat"); s.Space(flat) != Space2D {
		t.Fatal("root children default to the scene space")
	}
}

func TestSpaceMismatchRejected(t *testing.T) {
	s := spaceScene(t)
	p := s.Create(s.Root(), "p")
	if s.AllowsSpace(p, Space3D) {
		t.Fatal("2D parent must not allow a 3D child")
	}
	h, err := s.CreateIn(p, "x", ids.NodeID{}, Space3D)
	if err == nil || !h.IsZero() {
		t.Fatalf("3D node under a 2D parent: handle %v err %v", h, err)
	}
}

func TestMixedWorldTransform(t *testing.T) {
	s := spaceScene(t)
	a := s.Create3D(s.Root(), "a")
	s.SetLocal3D(a, Transform3D{
		Position: mathx.Vec3{X: 1, Y: 2, Z: 3},
		Rotation: mathx.QuatAxisAngle(mathx.Vec3{Y: 1}, mathx.Pi/2),
		Scale:    mathx.Vec3{X: 1, Y: 1, Z: 1},
	})
	c := createIn(t, s, a, "c", Space2D)
	s.SetPosition(c, mathx.Vec2{X: 2, Y: 1})
	g := createIn(t, s, c, "g", Space2D)
	s.SetPosition(g, mathx.Vec2{X: 1})
	s.Flush()
	s.UpdateTransforms()
	want := s.World4(a).TransformPoint(mathx.Vec3{X: 2, Y: -1})
	if got := s.World4(c).TransformPoint(mathx.Vec3{}); !got.NearlyEqual(want, 1e-5) {
		t.Fatalf("2D under 3D: got %v want %v", got, want)
	}
	want = s.World4(a).TransformPoint(mathx.Vec3{X: 3, Y: -1})
	if got := s.World4(g).TransformPoint(mathx.Vec3{}); !got.NearlyEqual(want, 1e-5) {
		t.Fatalf("2D chain under 3D: got %v want %v", got, want)
	}
	s.SetPosition3D(a, mathx.Vec3{})
	s.UpdateTransforms()
	want = s.World4(a).TransformPoint(mathx.Vec3{X: 3, Y: -1})
	if got := s.World4(g).TransformPoint(mathx.Vec3{}); !got.NearlyEqual(want, 1e-5) {
		t.Fatalf("dirty propagation through the mixed chain: got %v want %v", got, want)
	}
}

func TestSetSpaceRules(t *testing.T) {
	s := spaceScene(t)
	a := s.Create3D(s.Root(), "a")
	s.Create(a, "child")
	if err := s.SetSpace(a, Space2D); err == nil {
		t.Fatal("space change on a node with children must fail")
	}
	leaf := s.Create(s.Root(), "leaf")
	s.SetPosition(leaf, mathx.Vec2{X: 4, Y: 2})
	if err := s.SetSpace(leaf, Space3D); err != nil {
		t.Fatal(err)
	}
	if p := s.Local3D(leaf).Position; !p.NearlyEqual(mathx.Vec3{X: 4, Y: -2}, 1e-5) {
		t.Fatalf("lift must map y to -y: %v", p)
	}
}
