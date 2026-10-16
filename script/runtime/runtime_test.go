package runtime

import (
	"errors"
	"strings"
	"testing"

	"aego/engine"
	scriptstdlib "aego/modules/scripting/stdlib"
	"aego/platform/input"
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
	"aego/script/vm"
)

const program = `package game

script Mover {
	@export var Speed float = 2
	var readies int

	func Ready() {
		readies++
	}

	func Update(dt float) {
		p := node.Position()
		p.X += Speed * dt
		node.SetPosition(p)
		if node.Has<Marker>() {
			m := node.Get<Marker>()
			m.FlipX = readies == 1
		}
	}
}

script Spinner {
	func Update(dt float) {
		for {
		}
	}
}
`

func setup(t *testing.T) (*scene.Types, *bind.Registry, *bytecode.Module) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	if err := registerMarker(types); err != nil {
		t.Fatal(err)
	}
	if err := scriptcomp.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	reg := bind.New()
	scriptstdlib.Install(reg, stdlib.Options{Seed: 1})
	BindComponents(reg, types.Schema())
	if err := reg.Freeze(); err != nil {
		t.Fatal(err)
	}
	sm := source.NewManager()
	f, ds := parser.ParseSource(sm, "game/main.aes", []byte(program))
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
	return types, reg, mod
}

func attach(s *scene.Scene, name, typ string, props schema.PropertyBag) scene.NodeHandle {
	h := s.Create(s.Root(), name)
	sc := scene.Add[scriptcomp.ScriptComponent](s, h)
	sc.Attachments = append(sc.Attachments, scriptcomp.Attachment{TypeName: typ, Enabled: true, Props: props})
	return h
}

func TestRuntimeEndToEnd(t *testing.T) {
	types, reg, mod := setup(t)
	rt := New(Options{Bindings: reg, Schema: types.Schema(), VM: vm.Options{Budget: 10_000}})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		t.Fatal(err)
	}
	if _, ok := types.Schema().ByName("game.Mover"); !ok {
		t.Fatal("Link did not publish script types")
	}
	s := scene.New(types, nil, scene.Options{})
	var props schema.PropertyBag
	props.Set("Speed", schema.Float32(4))
	mover := attach(s, "Mover", "game.Mover", props)
	*scene.Add[Marker](s, mover) = Marker{}
	attach(s, "Spinner", "game.Spinner", schema.PropertyBag{})
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}

	w := engine.NewWorld("test")
	w.Input = &input.State{}
	for frame := uint64(1); frame <= 2; frame++ {
		rt.Update(&engine.Context{World: w, Dt: 0.5, Frame: frame})
	}

	if got := s.Local(mover).Position.X; got != 4 {
		t.Fatalf("mover x = %v, want 4 after two 0.5s frames at speed 4", got)
	}
	if m, _ := scene.Get[Marker](s, mover); !m.FlipX {
		t.Fatal("script did not write the Go component through node.Get<Marker>(), or Ready ran more than once")
	}
	if n := len(rt.Faults()); n != 1 || rt.Faults()[0].Fault.Code != vm.CodeBudget {
		t.Fatalf("faults = %+v, want one budget fault from Spinner", rt.Faults())
	}
}

func TestRuntimeDestroyRemovesInstance(t *testing.T) {
	types, reg, mod := setup(t)
	rt := New(Options{Bindings: reg, Schema: types.Schema(), VM: vm.Options{Budget: 10_000}})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := attach(s, "Mover", "game.Mover", schema.PropertyBag{})
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}
	w := engine.NewWorld("test")
	w.Input = &input.State{}
	rt.Update(&engine.Context{World: w, Dt: 0.1, Frame: 1})
	s.Destroy(h)
	s.Flush()
	rt.Update(&engine.Context{World: w, Dt: 0.1, Frame: 2})
	rt.Update(&engine.Context{World: w, Dt: 0.1, Frame: 3})
	if st := rt.Stats(); st.Instances != 0 || st.Update != 0 {
		t.Fatalf("destroyed node still has %d instances and %d scheduled updates", st.Instances, st.Update)
	}
}

func TestComponentRefOnDestroyedNode(t *testing.T) {
	types, reg, mod := setup(t)
	rt := New(Options{Bindings: reg, Schema: types.Schema()})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	h := s.Create(s.Root(), "N")
	*scene.Add[Marker](s, h) = Marker{}
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}
	ref := vm.Value{A: bind.NodeValue(h).A, B: 0}
	var v vm.Value
	if err := rt.GetField(ref, 0, &v); err != nil {
		t.Fatalf("live component: %v", err)
	}
	s.Destroy(h)
	err := rt.GetField(ref, 0, &v)
	var fl *vm.Fault
	if !errors.As(err, &fl) || fl.Code != vm.CodeNil {
		t.Fatalf("destroyed node: %v", err)
	}
}

func TestComponentFieldLimit(t *testing.T) {
	td := &schema.TypeDescriptor{FullName: "game.Big", Owner: "game"}
	if err := fieldLimit(td, 255); err != nil {
		t.Fatal(err)
	}
	err := fieldLimit(td, 256)
	if err == nil || !strings.Contains(err.Error(), "game.Big") || !strings.Contains(err.Error(), "module game") {
		t.Fatalf("err = %v", err)
	}
}
