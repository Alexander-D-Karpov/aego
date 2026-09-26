package commands

import (
	"testing"

	"aego/core/errs"
	"aego/core/mathx"
	"aego/editor/history"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type Health struct {
	Max float32 `aego:"max"`
}

type Link struct {
	Target scene.NodeRef `aego:"target"`
}

type target struct {
	s *scene.Scene
}

func (t target) Scene() *scene.Scene {
	return t.s
}

func newScene(t *testing.T) *scene.Scene {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Health](types, schema.Name("test.Health")); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[Link](types, schema.Name("test.Link")); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return scene.New(types, nil, scene.Options{})
}

func TestDeleteUndoRestoresIDsAndRefs(t *testing.T) {
	s := newScene(t)
	first := s.Create(s.Root(), "First")
	a := s.Create(s.Root(), "A")
	child := s.Create(a, "Child")
	b := s.Create(s.Root(), "B")
	scene.Add[Health](s, a).Max = 42
	scene.Add[Link](s, b).Target.Set(s, child)
	s.Flush()
	aID, childID := s.ID(a), s.ID(child)

	h := history.New(target{s})
	if err := h.Exec(&Delete{Node: aID}); err != nil {
		t.Fatal(err)
	}
	if !s.Handle(aID).IsZero() || !s.Handle(childID).IsZero() {
		t.Fatal("subtree not deleted")
	}
	link, _ := scene.Get[Link](s, b)
	if _, ok := link.Target.Resolve(s); ok {
		t.Fatal("ref to a deleted node resolved")
	}
	if err := h.Undo(); err != nil {
		t.Fatal(err)
	}
	ra := s.Handle(aID)
	if ra.IsZero() || s.Parent(ra) != s.Root() || ChildIndex(s, ra) != 1 {
		t.Fatalf("restored at index %d", ChildIndex(s, ra))
	}
	if s.Handle(childID).IsZero() {
		t.Fatal("child id changed")
	}
	if hp, ok := scene.Get[Health](s, ra); !ok || hp.Max != 42 {
		t.Fatal("component data lost")
	}
	link, _ = scene.Get[Link](s, b)
	if got, ok := link.Target.Resolve(s); !ok || got != s.Handle(childID) {
		t.Fatal("node ref did not resolve after undo")
	}
	if ChildIndex(s, first) != 0 {
		t.Fatal("sibling order changed")
	}
}

func TestReparentPreservesWorld(t *testing.T) {
	s := newScene(t)
	p := s.Create(s.Root(), "P")
	n := s.Create(s.Root(), "N")
	s.SetLocal(p, scene.Transform2D{Position: mathx.Vec2{X: 10, Y: 5}, Rotation: 0.7, Scale: mathx.Vec2{X: 2, Y: 2}})
	s.SetLocal(n, scene.Transform2D{Position: mathx.Vec2{X: 3, Y: -4}, Rotation: 0.2, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.Flush()
	s.UpdateTransforms()
	before := s.World(n)

	h := history.New(target{s})
	if err := h.Exec(&Reparent{Node: s.ID(n), NewParent: s.ID(p), NewIndex: -1, KeepWorld: true}); err != nil {
		t.Fatal(err)
	}
	if s.Parent(n) != p {
		t.Fatal("not reparented")
	}
	s.UpdateTransforms()
	after := s.World(n)
	for i := range before {
		if mathx.Abs(before[i]-after[i]) > 1e-4 {
			t.Fatalf("world changed at %d: %v vs %v", i, before, after)
		}
	}
	h.Undo()
	if s.Parent(n) != s.Root() || s.Local(n).Position.X != 3 {
		t.Fatal("undo did not restore parent and local transform")
	}
}

func TestReparentUnderZeroScaleFails(t *testing.T) {
	s := newScene(t)
	p := s.Create(s.Root(), "P")
	n := s.Create(s.Root(), "N")
	s.SetLocal(p, scene.Transform2D{Scale: mathx.Vec2{X: 0, Y: 1e-9}})
	s.Flush()
	h := history.New(target{s})
	err := h.Exec(&Reparent{Node: s.ID(n), NewParent: s.ID(p), NewIndex: -1, KeepWorld: true})
	if !errs.Has(err, errs.SceneBadReparent) {
		t.Fatalf("err = %v", err)
	}
	if s.Parent(n) != s.Root() || h.Cursor() != 0 {
		t.Fatal("failed reparent changed the scene or history")
	}
}

func TestSetPropertyMergesInTransaction(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	scene.Add[Health](s, a).Max = 1
	s.Flush()
	typeID := scene.Store[Health](s).Type()
	fd, _ := s.Registry().Field(typeID, "max")

	h := history.New(target{s})
	tx := h.Begin("drag")
	prev := schema.Float32(1)
	for i := 2; i <= 5; i++ {
		v := schema.Float32(float32(i))
		tx.Exec(&SetProperty{Node: s.ID(a), Type: typeID, Field: fd.ID, Before: prev, After: v, Name: "max"})
		prev = v
	}
	tx.Commit()
	if tx.Len() != 1 || h.Cursor() != 1 {
		t.Fatalf("commands=%d cursor=%d", tx.Len(), h.Cursor())
	}
	h.Undo()
	if hp, _ := scene.Get[Health](s, a); hp.Max != 1 {
		t.Fatalf("max = %v", hp.Max)
	}
}

func TestCreateAndComponentRoundTrip(t *testing.T) {
	src := newScene(t)
	proto := src.Create(src.Root(), "Enemy")
	scene.Add[Health](src, proto).Max = 7
	src.Flush()
	fragment := sceneio.SaveSubtree(src, proto, false)

	s := newScene(t)
	h := history.New(target{s})
	c := &Create{Index: -1, Fragment: fragment, Name: "Enemy"}
	if err := h.Exec(c); err != nil {
		t.Fatal(err)
	}
	if len(c.Roots()) != 1 {
		t.Fatalf("roots = %d", len(c.Roots()))
	}
	n := s.Handle(c.Roots()[0])
	typeID := scene.Store[Health](s).Type()
	if err := h.Exec(&RemoveComponent{Node: s.ID(n), Type: typeID, Name: "Health"}); err != nil {
		t.Fatal(err)
	}
	if scene.Has[Health](s, n) {
		t.Fatal("component not removed")
	}
	h.Undo()
	if hp, ok := scene.Get[Health](s, n); !ok || hp.Max != 7 {
		t.Fatal("component data not restored")
	}
	h.Undo()
	if !s.Handle(c.Roots()[0]).IsZero() {
		t.Fatal("create not undone")
	}
	h.Redo()
	if s.Handle(c.Roots()[0]).IsZero() {
		t.Fatal("redo must recreate the same node id")
	}
}
