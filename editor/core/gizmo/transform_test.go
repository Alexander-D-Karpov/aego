package gizmo

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/history"
	"aego/scene"
	"aego/schema"
)

type target struct{ s *scene.Scene }

func (t target) Scene() *scene.Scene { return t.s }

func newNode(t *testing.T, pos mathx.Vec2) (*scene.Scene, ids.NodeID) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "N")
	s.SetLocal(h, scene.Transform2D{Position: pos, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.Flush()
	s.UpdateTransforms()
	return s, s.ID(h)
}

func TestMoveAxisWithSnap(t *testing.T) {
	s, id := newNode(t, mathx.Vec2{})
	g := &Transform{Mode: ModeMove}
	e := NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{}, Snap{Enabled: true, Move: 0.5})
	g.Begin(e, Hit{Part: PartX})
	e.Mouse = mathx.Vec2{X: 1.3, Y: 0.9}
	g.Update(e)
	if got := s.Local(s.Handle(id)).Position; !got.NearlyEqual(mathx.Vec2{X: 1.5}, 1e-5) {
		t.Fatalf("position = %v", got)
	}
}

func TestRotateAroundOwnPivot(t *testing.T) {
	s, id := newNode(t, mathx.Vec2{X: 2})
	g := &Transform{Mode: ModeRotate}
	e := NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{X: 3}, Snap{})
	g.Begin(e, Hit{Part: PartRotate})
	e.Mouse = mathx.Vec2{X: 2, Y: 1}
	g.Update(e)
	local := s.Local(s.Handle(id))
	if mathx.Abs(local.Rotation-mathx.Pi/2) > 1e-4 {
		t.Fatalf("rotation = %v", local.Rotation)
	}
	if !local.Position.NearlyEqual(mathx.Vec2{X: 2}, 1e-4) {
		t.Fatalf("position moved: %v", local.Position)
	}
}

func TestScaleX(t *testing.T) {
	s, id := newNode(t, mathx.Vec2{})
	g := &Transform{Mode: ModeScale}
	e := NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{X: 1}, Snap{})
	g.Begin(e, Hit{Part: PartScaleX})
	e.Mouse = mathx.Vec2{X: 2, Y: 0.4}
	g.Update(e)
	if sc := s.Local(s.Handle(id)).Scale; !sc.NearlyEqual(mathx.Vec2{X: 2, Y: 1}, 1e-4) {
		t.Fatalf("scale = %v", sc)
	}
}

func TestCommitIsOneUndoStep(t *testing.T) {
	s, id := newNode(t, mathx.Vec2{})
	h := history.New(target{s})
	g := &Transform{Mode: ModeMove}
	e := NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{}, Snap{})
	e.Tx = h.Begin("Move")
	g.Begin(e, Hit{Part: PartXY})
	for i := 1; i <= 10; i++ {
		e.Mouse = mathx.Vec2{X: float32(i)}
		g.Update(e)
	}
	g.End(e)
	e.Tx.Commit()
	if h.Cursor() != 1 {
		t.Fatalf("cursor = %d", h.Cursor())
	}
	h.Undo()
	if got := s.Local(s.Handle(id)).Position; got != (mathx.Vec2{}) {
		t.Fatalf("undo left %v", got)
	}
	h.Redo()
	if got := s.Local(s.Handle(id)).Position; !got.NearlyEqual(mathx.Vec2{X: 10}, 1e-5) {
		t.Fatalf("redo gave %v", got)
	}
}

func TestCancelRestores(t *testing.T) {
	s, id := newNode(t, mathx.Vec2{X: 1})
	g := &Transform{Mode: ModeMove}
	e := NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{}, Snap{})
	g.Begin(e, Hit{Part: PartXY})
	e.Mouse = mathx.Vec2{X: 5}
	g.Update(e)
	Cancel(e)
	if got := s.Local(s.Handle(id)).Position; got != (mathx.Vec2{X: 1}) {
		t.Fatalf("cancel left %v", got)
	}
}
