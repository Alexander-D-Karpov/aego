package scene

import (
	"testing"

	"aego/core/mathx"
)

const eps = 1e-4

func TestWorldIsParentTimesLocal(t *testing.T) {
	s := newScene(t)
	parent := s.Create(s.Root(), "Parent")
	child := s.Create(parent, "Child")
	s.Flush()

	s.SetLocal(parent, Transform2D{
		Position: mathx.Vec2{X: 10, Y: 20},
		Rotation: mathx.Pi / 2,
		Scale:    mathx.Vec2{X: 2, Y: 2},
	})
	s.SetLocal(child, Transform2D{
		Position: mathx.Vec2{X: 1, Y: 0},
		Scale:    mathx.Vec2{X: 1, Y: 1},
	})
	s.UpdateTransforms()

	got := s.WorldPosition(child)
	if !got.NearlyEqual(mathx.Vec2{X: 10, Y: 22}, eps) {
		t.Fatalf("world position = %v", got)
	}
	want := s.World(parent).Mul(s.Local(child).Matrix())
	have := s.World(child)
	for i := range want {
		if mathx.Abs(want[i]-have[i]) > eps {
			t.Fatalf("world matrix mismatch at %d: %v vs %v", i, have, want)
		}
	}
}

func TestDirtyPropagatesToSubtreeOnly(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	aChild := s.Create(a, "AChild")
	b := s.Create(s.Root(), "B")
	s.Flush()
	s.UpdateTransforms()

	before := s.World(b)
	s.SetPosition(a, mathx.Vec2{X: 5})

	an, _ := s.nodes.Get(a)
	cn, _ := s.nodes.Get(aChild)
	bn, _ := s.nodes.Get(b)
	if !an.has(flagWorldDirty) || !cn.has(flagWorldDirty) {
		t.Fatal("subtree must be marked dirty")
	}
	if bn.has(flagWorldDirty) {
		t.Fatal("unrelated node must stay clean")
	}
	s.UpdateTransforms()
	if s.World(b) != before {
		t.Fatal("unrelated world matrix changed")
	}
	if !s.WorldPosition(aChild).NearlyEqual(mathx.Vec2{X: 5}, eps) {
		t.Fatalf("child world = %v", s.WorldPosition(aChild))
	}
}

func TestWorldAccessorUpdatesOnDemand(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()
	s.SetPosition(a, mathx.Vec2{X: 3, Y: 4})
	if got := s.WorldPosition(a); !got.NearlyEqual(mathx.Vec2{X: 3, Y: 4}, eps) {
		t.Fatalf("World must refresh a dirty node: %v", got)
	}
}

func TestEffectiveEnabled(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(a, "B")
	c := s.Create(b, "C")
	s.Flush()
	s.UpdateTransforms()

	if !s.EffectiveEnabled(c) {
		t.Fatal("nodes must start enabled")
	}
	s.SetEnabled(a, false)
	s.UpdateTransforms()
	if s.EffectiveEnabled(b) || s.EffectiveEnabled(c) {
		t.Fatal("disabling a parent must disable the subtree")
	}
	if !s.Enabled(b) {
		t.Fatal("local enabled flag must be untouched")
	}
	s.SetEnabled(a, true)
	s.UpdateTransforms()
	if !s.EffectiveEnabled(c) {
		t.Fatal("re-enabling must restore the subtree")
	}
	s.SetEnabled(b, false)
	s.UpdateTransforms()
	if s.EffectiveEnabled(c) || !s.EffectiveEnabled(a) {
		t.Fatal("disabling a middle node must only affect its subtree")
	}
}

func TestDestroyedNodeIsNotEffective(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()
	s.UpdateTransforms()
	s.Destroy(a)
	if s.EffectiveEnabled(a) {
		t.Fatal("a destroyed node must not be effective before Flush")
	}
}
