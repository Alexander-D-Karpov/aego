package editor

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/gizmo"
	"aego/editor/core/history"
	"aego/engine/module"
	"aego/modules/physics"
	"aego/modules/physics/components"
	"aego/scene"
	"aego/schema"
)

type target struct {
	s *scene.Scene
}

func (t target) Scene() *scene.Scene {
	return t.s
}

func fixture(t *testing.T) (*scene.Scene, *plugin) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if _, err := module.Load(types, nil, physics.New()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	p, err := newPlugin(nil, types.Schema())
	if err != nil {
		t.Fatal(err)
	}
	return scene.New(types, nil, scene.Options{}), p
}

func drag(t *testing.T, s *scene.Scene, h scene.NodeHandle, g gizmo.Gizmo, part int, to mathx.Vec2) *history.History {
	t.Helper()
	hist := history.New(target{s})
	id := s.ID(h)
	e := gizmo.NewEdit(s, []ids.NodeID{id}, id, mathx.Vec2{}, gizmo.Snap{})
	e.Tx = hist.Begin("Edit")
	g.Begin(e, gizmo.Hit{Part: gizmo.Part(partBase + part)})
	e.Mouse = to
	g.Update(e)
	g.End(e)
	if e.Err != nil {
		t.Fatal(e.Err)
	}
	e.Tx.Commit()
	return hist
}

func TestBoxSideDragAndUndo(t *testing.T) {
	s, p := fixture(t)
	h := s.Create(s.Root(), "Box")
	*scene.Add[components.BoxCollider2D](s, h) = components.DefaultBox()
	s.Flush()
	g := &colliderGizmo{p: p, kind: components.KindBox, td: p.types[components.KindBox]}
	hist := drag(t, s, h, g, 4, mathx.Vec2{X: 1.5})
	b, _ := scene.Get[components.BoxCollider2D](s, h)
	if b.Size != (mathx.Vec2{X: 2, Y: 1}) || b.Offset != (mathx.Vec2{X: 0.5}) {
		t.Fatalf("box %v %v", b.Size, b.Offset)
	}
	if err := hist.Undo(); err != nil {
		t.Fatal(err)
	}
	b, _ = scene.Get[components.BoxCollider2D](s, h)
	if b.Size != (mathx.Vec2{X: 1, Y: 1}) || b.Offset != (mathx.Vec2{}) {
		t.Fatalf("undo left %v %v", b.Size, b.Offset)
	}
}

func TestPolygonInsertVertex(t *testing.T) {
	s, p := fixture(t)
	h := s.Create(s.Root(), "Poly")
	*scene.Add[components.PolygonCollider2D](s, h) = components.DefaultPolygon()
	s.Flush()
	g := &colliderGizmo{p: p, kind: components.KindPolygon, td: p.types[components.KindPolygon]}
	hist := drag(t, s, h, g, 4, mathx.Vec2{Y: -1})
	c, _ := scene.Get[components.PolygonCollider2D](s, h)
	if len(c.Points) != 5 || c.Points[1] != (mathx.Vec2{Y: -1}) {
		t.Fatalf("points %v", c.Points)
	}
	hist.Undo()
	c, _ = scene.Get[components.PolygonCollider2D](s, h)
	if len(c.Points) != 4 {
		t.Fatalf("undo left %v", c.Points)
	}
}

func TestConcavePolygonIsReported(t *testing.T) {
	s, _ := fixture(t)
	h := s.Create(s.Root(), "Poly")
	c := scene.Add[components.PolygonCollider2D](s, h)
	*c = components.DefaultPolygon()
	c.Points = []mathx.Vec2{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 0, Y: -0.2}, {X: 1, Y: 1}, {X: -1, Y: 1}}
	s.Flush()
	found := module.ValidateScene(s, nil)
	if len(found) != 1 || found[0].Owner != physics.ID || found[0].Subject.Field != components.NamePolygon || !found[0].BlocksPlay {
		t.Fatalf("issues %+v", found)
	}
}
