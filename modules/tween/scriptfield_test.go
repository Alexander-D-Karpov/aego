package tween

import (
	"testing"

	"aego/contracts/scriptfields"
	"aego/core/mathx"
	"aego/engine"
	"aego/scene"
	"aego/script/abi"
	"aego/script/vm"
)

type fakeFields struct {
	node  scene.NodeHandle
	kinds map[string]abi.Kind
	vals  map[string]vm.Value
}

func (f *fakeFields) ResolveField(node scene.NodeHandle, script, field string) (scriptfields.Ref, bool) {
	k, ok := f.kinds[field]
	if !ok || node != f.node || (script != "Mover" && script != "game.Mover") {
		return scriptfields.Ref{}, false
	}
	return scriptfields.Ref{Node: node, Script: "game.Mover", Field: field, Kind: k}, true
}

func (f *fakeFields) LoadField(ref *scriptfields.Ref) (vm.Value, bool) {
	v, ok := f.vals[ref.Field]
	return v, ok
}

func (f *fakeFields) StoreField(ref *scriptfields.Ref, v vm.Value) bool {
	if _, ok := f.vals[ref.Field]; !ok {
		return false
	}
	f.vals[ref.Field] = v
	return true
}

func scriptSetup(t *testing.T) (*Service, *fakeFields, scene.NodeHandle) {
	t.Helper()
	s := newScene(t)
	h := s.Create(s.Root(), "n")
	s.Flush()
	f := &fakeFields{
		node:  h,
		kinds: map[string]abi.Kind{"Speed": abi.Float, "Vel": abi.Vec2, "Hits": abi.Int, "Name": abi.String},
		vals:  map[string]vm.Value{"Speed": vm.Float(0), "Vel": vm.Vec(1, 1, 0, 0), "Hits": vm.Int(0), "Name": {}},
	}
	w := engine.NewWorld("t")
	w.SetService(scriptfields.ServiceID, scriptfields.Fields(f))
	return NewService(s, w), f, h
}

func TestScriptFieldTargets(t *testing.T) {
	svc, f, h := scriptSetup(t)
	if _, err := svc.ToFloat(h, "script/Mover.Speed", 4, 0.25, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToFloat(h, "script/game.Mover.Vel.y", -3, 0.25, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToFloat(h, "script/Mover.Hits", 3, 0.25, Options{}); err != nil {
		t.Fatal(err)
	}
	steps(svc, 8)
	if v := f.vals["Hits"].Int(); v != 2 {
		t.Fatalf("an int script field rounds the halfway value, got %d", v)
	}
	steps(svc, 8)
	if v := f.vals["Speed"].Float(); v != 4 {
		t.Fatalf("Speed %v", v)
	}
	if v := f.vals["Vel"]; v.Lane(0) != 1 || v.Lane(1) != -3 || v.Lane(2) != 0 || v.Lane(3) != 0 {
		t.Fatalf("Vel lanes %v %v %v %v", v.Lane(0), v.Lane(1), v.Lane(2), v.Lane(3))
	}
	if f.vals["Hits"].Int() != 3 {
		t.Fatal("int end value")
	}
}

func TestScriptFieldErrors(t *testing.T) {
	svc, f, h := scriptSetup(t)
	if _, err := svc.ToFloat(h, "script/Mover.Name", 1, 1, Options{}); err == nil {
		t.Fatal("a string script field must not tween")
	}
	if _, err := svc.ToFloat(h, "script/Other.Speed", 1, 1, Options{}); err == nil {
		t.Fatal("unknown script accepted")
	}
	if _, err := svc.ToVec2(h, "script/Mover.Speed", mathx.Vec2{}, 1, Options{}); err == nil {
		t.Fatal("a Vec2 tween on a float field must fail")
	}
	tw, err := svc.ToFloat(h, "script/Mover.Speed", 4, 1, Options{})
	if err != nil {
		t.Fatal(err)
	}
	steps(svc, 4)
	delete(f.vals, "Speed")
	steps(svc, 1)
	if !svc.Done(tw) {
		t.Fatal("a tween whose script field is gone must stop")
	}
}

func TestScriptFieldsWithoutScripts(t *testing.T) {
	svc, _, h := setup(t)
	if _, err := svc.ToFloat(h, "script/Mover.Speed", 1, 1, Options{}); err == nil {
		t.Fatal("script targets need the script runtime")
	}
}
