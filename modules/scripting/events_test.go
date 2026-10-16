package scripting_test

import (
	"testing"

	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/modules/scripting"
	scriptsystems "aego/modules/scripting/systems"
	"aego/scene"
	"aego/scene/scriptcomp"
	"aego/script/abi"
	"aego/script/ast"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/checker"
	"aego/script/compiler"
	"aego/script/diag"
	"aego/script/parser"
	"aego/script/runtime"
	"aego/script/source"
	"aego/script/stdlib"
	"aego/script/vm"
)

type evModule struct {
	id   string
	name string
	out  *scriptevents.EventID
}

func (m evModule) Info() module.Info {
	return module.Info{ID: m.id, Version: "1"}
}

func (m evModule) Register(r *module.Registry) error {
	id := r.ScriptEvent(scriptevents.Event{Name: m.name, Params: []abi.TypeRef{abi.T(abi.Int)}, Names: []string{"n"}})
	if m.out != nil {
		*m.out = id
	}
	return nil
}

type rig struct {
	types *scene.Types
	bind  *bind.Registry
	ev    scriptevents.EventID
}

func newRig(t *testing.T) *rig {
	types, err := base.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{types: types, bind: bind.New()}
	if _, err := module.Load(types, r.bind, base.Core{}, scripting.New(stdlib.Options{Seed: 1}), evModule{id: "evtest", name: "OnHit", out: &r.ev}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	if err := scriptsystems.Freeze(r.bind, types.Schema()); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *rig) compile(src string) (*bytecode.Module, []diag.Diagnostic) {
	sm := source.NewManager()
	id := sm.Add("scripts/evt/main.aes", []byte(src))
	f, ds := parser.ParseFile(sm, id)
	a := checker.Check(sm, map[string][]*ast.File{"evt": {f}}, r.bind.Surface(), checker.Options{Module: "evt"})
	all := append(ds, a.Diagnostics...)
	if diag.HasErrors(all) {
		return nil, all
	}
	mod, cds := compiler.Compile(a, "evt", compiler.Options{Sources: sm})
	return mod, append(all, cds...)
}

func hasCode(ds []diag.Diagnostic, code string) bool {
	for _, d := range ds {
		if d.Code == code {
			return true
		}
	}
	return false
}

const targetSrc = `package evt

script Target {
	var hits int
	var order int

	func OnHit(n int) {
		hits++
		order = order*10 + n
	}
}

script Plain {
	var x int
}
`

func TestEventHandlerIsLifecycle(t *testing.T) {
	r := newRig(t)
	mod, ds := r.compile(targetSrc)
	if mod == nil {
		t.Fatalf("compile: %v", ds)
	}
	if ev := mod.Scripts[0].Events; len(ev) != 1 || ev[0].Name != "OnHit" {
		t.Fatalf("events %+v", ev)
	}
	cases := map[string]string{
		diag.EventSignature:     "package evt\nscript S {\n\tfunc OnHit(n float) {}\n}\n",
		diag.EventCall:          "package evt\nscript S {\n\tfunc OnHit(n int) {}\n\tfunc Update(dt float) {\n\t\tOnHit(1)\n\t}\n}\n",
		diag.CoroutineLifecycle: "package evt\nscript S {\n\tfunc OnHit(n int) coroutine {\n\t\tyield\n\t}\n}\n",
	}
	for code, src := range cases {
		if _, ds := r.compile(src); !hasCode(ds, code) {
			t.Fatalf("%s not reported: %v", code, ds)
		}
	}
}

func TestEventRegistrationErrors(t *testing.T) {
	for _, name := range []string{"Update", "onHit", ""} {
		types, _ := base.NewTypes()
		if _, err := module.Load(types, bind.New(), base.Core{}, scripting.New(stdlib.Options{}), evModule{id: "evtest", name: name}); err == nil {
			t.Fatalf("event %q accepted", name)
		}
	}
	types, _ := base.NewTypes()
	if _, err := module.Load(types, bind.New(), base.Core{}, scripting.New(stdlib.Options{}), evModule{id: "a", name: "OnHit"}, evModule{id: "b", name: "OnHit"}); err == nil {
		t.Fatal("duplicate event accepted")
	}
}

func attach(s *scene.Scene, h scene.NodeHandle, typ string) {
	sc := scene.Add[scriptcomp.ScriptComponent](s, h)
	sc.Attachments = append(sc.Attachments, scriptcomp.Attachment{TypeName: typ, Enabled: true})
}

func TestEventDispatch(t *testing.T) {
	r := newRig(t)
	mod, ds := r.compile(targetSrc)
	if mod == nil {
		t.Fatalf("compile: %v", ds)
	}
	rt := runtime.New(runtime.Options{Bindings: r.bind, Schema: r.types.Schema()})
	if err := rt.Link([]*bytecode.Module{mod}); err != nil {
		t.Fatal(err)
	}
	s := scene.New(r.types, nil, scene.Options{})
	target := s.Create(s.Root(), "Target")
	plain := s.Create(s.Root(), "Plain")
	attach(s, target, "evt.Target")
	attach(s, plain, "evt.Plain")
	s.Flush()
	if err := rt.Instantiate(s); err != nil {
		t.Fatal(err)
	}
	ec := &engine.Context{Frame: 1, Dt: 1.0 / 60}
	rt.DispatchEvents(ec)
	id, ok := rt.InstanceOf(target, "evt.Target")
	if !ok {
		t.Fatal("no Target instance")
	}
	field := func(name string) int64 {
		v, _ := rt.Field(id, name)
		return v.Int()
	}
	for _, n := range []int64{1, 2, 3} {
		rt.Emit(target, r.ev, []vm.Value{vm.Int(n)})
	}
	if field("hits") != 0 {
		t.Fatal("Emit called the handler before Flush")
	}
	rt.DispatchEvents(ec)
	if field("hits") != 3 || field("order") != 123 {
		t.Fatalf("hits %d order %d", field("hits"), field("order"))
	}
	calls := rt.EventCalls()
	rt.Emit(plain, r.ev, []vm.Value{vm.Int(9)})
	rt.Flush()
	if rt.EventCalls() != calls {
		t.Fatal("an instance without a handler was visited")
	}
	args := []vm.Value{vm.Int(1)}
	allocs := testing.AllocsPerRun(10, func() {
		for i := 0; i < 1000; i++ {
			rt.Emit(target, r.ev, args)
		}
		rt.Flush()
	})
	if allocs != 0 {
		t.Fatalf("1000 events allocate %v times", allocs)
	}
}
