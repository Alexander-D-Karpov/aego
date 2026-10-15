package bind_test

import (
	"errors"
	"testing"

	"aego/scene"
	"aego/schema"
	"aego/script/abi"
	"aego/script/bind"
	"aego/script/bytecode"
	"aego/script/vm"
)

type Counter struct {
	N int64 `aego:"n"`
}

func counterHost(t *testing.T) (*scene.Scene, vm.HostFunc, *bind.Registry) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Counter](types, schema.Name("test.Counter")); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	td, _ := types.Schema().ByName("test.Counter")
	r := bind.New()
	bind.MethodProc0(r.ComponentMethods("test.Counter"), "Bump", td.ID, func(_ *bind.Ctx, c *Counter) { c.N++ })
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	fn, ok := r.Resolve(bytecode.HostImport{Module: "#test.Counter", Name: "Bump", Sig: abi.Sig(abi.T(abi.Void), abi.ComponentOf("test.Counter"))})
	if !ok {
		t.Fatal("component method does not resolve")
	}
	return scene.New(types, nil, scene.Options{}), fn, r
}

func TestComponentMethodTable(t *testing.T) {
	_, _, r := counterHost(t)
	sig, ok := r.Table().ComponentMethod("test.Counter", "Bump")
	if !ok || sig.String() != "()" {
		t.Fatalf("sig %v %v", sig, ok)
	}
	if len(r.Table().FuncList()) != 0 {
		t.Fatal("component methods must not appear as module functions")
	}
}

func TestComponentMethodDeadRef(t *testing.T) {
	s, fn, _ := counterHost(t)
	live := s.Create(s.Root(), "Live")
	scene.Add[Counter](s, live)
	bare := s.Create(s.Root(), "Bare")
	gone := s.Create(s.Root(), "Gone")
	scene.Add[Counter](s, gone)
	s.Destroy(gone)
	s.Flush()
	m := vm.New(vm.Options{})
	m.Env = &bind.Ctx{Scene: s}
	if err := fn(m, []vm.Value{bind.NodeValue(live)}); err != nil {
		t.Fatal(err)
	}
	if c, _ := scene.Get[Counter](s, live); c.N != 1 {
		t.Fatalf("count %d", c.N)
	}
	for _, h := range []scene.NodeHandle{gone, bare, {}} {
		err := fn(m, []vm.Value{bind.NodeValue(h)})
		var f *vm.Fault
		if !errors.As(err, &f) || f.Code != vm.CodeNil {
			t.Fatalf("handle %v: err %v", h, err)
		}
	}
}
