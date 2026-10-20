package commands

import (
	"testing"

	"aego/core/errs"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/scene"
)

func TestReparent3DPreservesWorld(t *testing.T) {
	s := newScene(t)
	p := s.Create3D(s.Root(), "P")
	n := s.Create3D(s.Root(), "N")
	s.SetLocal3D(p, scene.Transform3D{Position: mathx.Vec3{X: 1, Y: 2, Z: 3}, Rotation: mathx.QuatEuler(0.3, 0.5, 0), Scale: mathx.Vec3{X: 2, Y: 2, Z: 2}})
	s.SetLocal3D(n, scene.Transform3D{Position: mathx.Vec3{X: -2, Z: 1}, Rotation: mathx.QuatEuler(0, 0, 0.4), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}})
	s.Flush()
	before := s.World4(n)
	h := history.New(target{s})
	if err := h.Exec(reparent(s, n, p)); err != nil {
		t.Fatal(err)
	}
	if s.Parent(n) != p {
		t.Fatal("not reparented")
	}
	if !s.World4(n).NearlyEqual(before, 1e-4) {
		t.Fatal("world transform changed")
	}
	h.Undo()
	if s.Parent(n) != s.Root() || s.Local3D(n).Position != (mathx.Vec3{X: -2, Z: 1}) {
		t.Fatal("undo did not restore parent and pose")
	}
}

func TestReparent3DUnderTwoDFails(t *testing.T) {
	s := newScene(t)
	flat := s.Create(s.Root(), "Flat")
	n := s.Create3D(s.Root(), "N")
	s.Flush()
	h := history.New(target{s})
	err := h.Exec(reparent(s, n, flat))
	if !errs.Has(err, errs.SceneSpaceMismatch) {
		t.Fatalf("err = %v", err)
	}
	if s.Parent(n) != s.Root() || h.Cursor() != 0 {
		t.Fatal("failed reparent changed the scene or history")
	}
}

func TestReparent2DIntoThreeDKeepsWorld(t *testing.T) {
	s := newScene(t)
	p := s.Create3D(s.Root(), "P")
	s.SetLocal3D(p, scene.Transform3D{Position: mathx.Vec3{X: 4, Y: 1}, Rotation: mathx.QuatAxisAngle(mathx.Vec3{Z: 1}, 0.3), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}})
	n := s.Create(s.Root(), "N")
	s.SetLocal(n, scene.Transform2D{Position: mathx.Vec2{X: 2, Y: -3}, Rotation: 0.2, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.Flush()
	before := s.World4(n)
	h := history.New(target{s})
	if err := h.Exec(reparent(s, n, p)); err != nil {
		t.Fatal(err)
	}
	if !s.Under3D(n) || !s.World4(n).NearlyEqual(before, 1e-4) {
		t.Fatal("2d node moved when entering the 3d parent plane")
	}
}

func TestSetSpaceUndoRestoresTransform(t *testing.T) {
	s := newScene(t)
	n := s.Create(s.Root(), "N")
	orig := scene.Transform2D{Position: mathx.Vec2{X: 3, Y: 4}, Rotation: 0.5, Scale: mathx.Vec2{X: 2, Y: 1}}
	s.SetLocal(n, orig)
	s.Flush()
	h := history.New(target{s})
	if err := h.Exec(&SetSpace{Node: s.ID(n), After: scene.Space3D}); err != nil {
		t.Fatal(err)
	}
	if s.Space(n) != scene.Space3D || !s.Local3D(n).Position.NearlyEqual(mathx.Vec3{X: 3, Y: -4}, 1e-6) {
		t.Fatal("space change failed")
	}
	h.Undo()
	if s.Space(n) != scene.Space2D || s.Local(n) != orig {
		t.Fatalf("undo left %v %+v", s.Space(n), s.Local(n))
	}
	h.Redo()
	if s.Space(n) != scene.Space3D {
		t.Fatal("redo failed")
	}
}

func TestDeleteUndoKeepsSpace(t *testing.T) {
	s := newScene(t)
	n := s.Create3D(s.Root(), "N")
	tr := scene.Transform3D{Position: mathx.Vec3{X: 1, Y: 2, Z: 3}, Rotation: mathx.QuatEuler(0.2, 0.1, 0), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}
	s.SetLocal3D(n, tr)
	scene.Add[Health](s, n).Max = 3
	s.Flush()
	id := s.ID(n)
	h := history.New(target{s})
	if err := h.Exec(del(s, id)); err != nil {
		t.Fatal(err)
	}
	h.Undo()
	r := s.Handle(id)
	if r.IsZero() || s.Space(r) != scene.Space3D || s.Local3D(r) != tr {
		t.Fatalf("restored %v %+v", s.Space(r), s.Local3D(r))
	}
}
