package edit

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/history/commands"
	"aego/modules/physics/components"
	"aego/modules/render2d"
	"aego/scene"
)

func TestColliderFitsSprite(t *testing.T) {
	p := newProject(t)
	rel := "scenes/fit.aescene"
	writeScene(t, p, rel, func(s *scene.Scene) {
		n := s.Create(s.Root(), "Crate")
		sr := scene.Add[render2d.SpriteRenderer](s, n)
		*sr = render2d.DefaultSpriteRenderer()
		sr.Size = mathx.Vec2{X: 2, Y: 3}
		s.Create(s.Root(), "Bare")
	})
	s, err := Open(p, headless())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	es := d.Scene()
	td, _ := ctx.Types.Schema().ByName(components.NameBox)
	box := func(name string) components.BoxCollider2D {
		c, ok := scene.Get[components.BoxCollider2D](es, es.Find(es.Root(), name))
		if !ok {
			t.Fatalf("%s has no box", name)
		}
		return *c
	}
	for _, name := range []string{"Crate", "Bare"} {
		id := es.ID(es.Find(es.Root(), name))
		if err := d.History.Exec(&commands.AddComponent{Node: id, Type: td.ID, Name: td.FullName, OnAdd: ctx.OnAdd(td)}); err != nil {
			t.Fatal(err)
		}
	}
	if c := box("Crate"); c.Size != (mathx.Vec2{X: 2, Y: 3}) || c.Offset != (mathx.Vec2{}) {
		t.Fatalf("crate box %+v", c)
	}
	if c := box("Bare"); c.Size != (mathx.Vec2{X: 1, Y: 1}) || c.Density != 1 {
		t.Fatalf("bare box %+v", c)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := d.History.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, ok := scene.Get[components.BoxCollider2D](es, es.Find(es.Root(), "Crate")); ok {
		t.Fatal("undo kept the box")
	}
	if err := d.History.Redo(); err != nil {
		t.Fatal(err)
	}
	if c := box("Crate"); c.Size != (mathx.Vec2{X: 2, Y: 3}) {
		t.Fatalf("redo box %+v", c)
	}
}
