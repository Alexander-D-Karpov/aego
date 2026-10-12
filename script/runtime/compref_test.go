package runtime

import (
	"testing"

	"aego/engine"
	"aego/engine/module"
	"aego/platform/input"
	"aego/render2d"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/scriptcomp"
	"aego/schema"
	"aego/script/ast"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/source"
	"aego/script/stdlib"
)

const compRefProgram = `package game

script Painter {
	@export var Sprite ComponentRef<engine.SpriteRenderer>

	var Got int

	func Ready() {
		Got = Sprite.Order
	}
}
`

func TestExportedComponentRef(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, render2d.New()); err != nil {
		t.Fatal(err)
	}
	if err := scriptcomp.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	reg := bind.New()
	stdlib.Install(reg, stdlib.Options{Seed: 1})
	BindComponents(reg, types.Schema())
	if err := reg.Freeze(); err != nil {
		t.Fatal(err)
	}
	sm := source.NewManager()
	f, ds := parser.ParseSource(sm, "game/main.aes", []byte(compRefProgram))
	a := checker.Check(sm, map[string][]*ast.File{"game": {f}}, reg.Surface(), checker.Options{Module: "game"})
	ds = append(ds, a.Diagnostics...)
	mod, cds := compiler.Compile(a, "game", compiler.Options{Sources: sm})
	ds = append(ds, cds...)
	if mod == nil || diag.HasErrors(ds) {
		for _, d := range ds {
			t.Log(diag.Format(sm, d))
		}
		t.Fatal("program does not compile")
	}
	tds, err := Describe([]*bytecode.Module{mod}, reg.Surface())
	if err != nil {
		t.Fatal(err)
	}
	fd, ok := tds[0].Field("Sprite")
	if !ok || fd.Kind != schema.KindNodeRef || fd.Component != "engine.SpriteRenderer" {
		t.Fatalf("descriptor = %+v", fd)
	}

	rt := New(Options{Bindings: reg, Schema: types.Schema()})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	sprite := s.Create(s.Root(), "Sprite")
	sr := scene.Add[render2d.SpriteRenderer](s, sprite)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Order = 7
	bare := s.Create(s.Root(), "Bare")
	good := attach(s, "Good", "game.Painter", refTo(s, sprite, "Sprite"))
	bad := attach(s, "Bad", "game.Painter", refTo(s, bare, "Sprite"))
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}
	w := engine.NewWorld("test")
	w.Input = &input.State{}
	rt.Update(&engine.Context{World: w, Dt: 0.1, Frame: 1})
	id, _ := rt.InstanceOf(good, "game.Painter")
	if v, _ := rt.Field(id, "Got"); v.Int() != 7 {
		t.Fatalf("Got = %d", v.Int())
	}
	id, _ = rt.InstanceOf(bad, "game.Painter")
	if v, _ := rt.Field(id, "Sprite"); v.A != 0 {
		t.Fatalf("ref to a node without the component must start nil, got %+v", v)
	}
}
