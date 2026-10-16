package world

import "testing"

func determinismScene(t *testing.T) *World {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	addStatic(t, w, Vec2{X: 8, Y: -2}, 0.4, Box{Half: Vec2{X: 3, Y: 0.2}}, nil)
	for i := 0; i < 10; i++ {
		for k := 0; k < 10; k++ {
			addDynamic(t, w, Vec2{X: float32(k)*0.52 - 2.5, Y: -0.25 - float32(i)*0.52}, 0, Box{Half: Vec2{X: 0.25, Y: 0.25}}, nil)
		}
	}
	for i := 0; i < 12; i++ {
		addDynamic(t, w, Vec2{X: 6 + float32(i%3)*0.4, Y: -8 - float32(i)}, 0, Circle{R: 0.2}, func(_ *BodyDef, s *ShapeDef) { s.Restitution = 0.4 })
	}
	hub := must(w.CreateBody(BodyDef{Type: Static, Position: Vec2{X: -6, Y: -12}, Mask: ^uint32(0)}))
	prev := hub
	for i := 0; i < 8; i++ {
		link, _ := addDynamic(t, w, Vec2{X: -6 + 0.5*float32(i) + 0.25, Y: -12}, 0, Capsule{HalfLen: 0.25, R: 0.08}, nil)
		la := Vec2{X: 0.25}
		if prev == hub {
			la = Vec2{}
		}
		must(w.CreateJoint(RevoluteJoint{A: prev, B: link, LocalAnchorA: la, LocalAnchorB: Vec2{X: -0.25}}))
		prev = link
	}
	return w
}

func TestSnapshotsAreBitwiseEqual(t *testing.T) {
	steps := 1000
	if testing.Short() {
		steps = 300
	}
	a, b := determinismScene(t), determinismScene(t)
	run(a, steps)
	run(b, steps)
	if !a.Snapshot().Equal(b.Snapshot()) {
		t.Fatal("two identical worlds diverged")
	}
}

func TestRestoreReplays(t *testing.T) {
	w := determinismScene(t)
	run(w, 200)
	snap := w.Snapshot()
	run(w, 100)
	direct := w.Snapshot()
	w.Restore(snap)
	if !w.Snapshot().Equal(snap) {
		t.Fatal("restore did not reproduce the snapshot")
	}
	run(w, 100)
	if !w.Snapshot().Equal(direct) {
		t.Fatal("restore and replay diverged")
	}
}
