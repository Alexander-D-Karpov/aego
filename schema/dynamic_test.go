package schema

import (
	"testing"
	"unsafe"
)

func frozen(t *testing.T) *Registry {
	type Native struct {
		A int32 `aego:"a"`
	}
	r := NewRegistry()
	if _, err := Register[Native](r, Name("engine.Native"), FormerName("engine.Old")); err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSetDynamic(t *testing.T) {
	r := frozen(t)
	td, err := NewScriptType("game.Mover", []string{"game.OldMover"}, []FieldDescriptor{
		{Name: "Speed", Kind: KindFloat32, Former: []string{"Velocity"}, Default: Float32(2), HasDefault: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	notified := 0
	r.Subscribe(func() { notified++ })
	if err := r.SetDynamic([]*TypeDescriptor{td}); err != nil {
		t.Fatal(err)
	}
	if got, ok := r.ByName("game.OldMover"); !ok || got != td {
		t.Fatal("former name does not resolve")
	}
	if _, ok := r.ByID(td.ID); !ok || notified != 1 {
		t.Fatal("dynamic type is not visible by id or subscribers were not notified")
	}
	fd, ok := td.Field("Velocity")
	if !ok {
		t.Fatal("former field name does not resolve")
	}
	var bag PropertyBag
	var v Value
	fd.Get(unsafe.Pointer(&bag), &v)
	if v.F != 2 {
		t.Fatalf("default = %v, want 2", v.F)
	}
	if err := fd.Set(unsafe.Pointer(&bag), Float32(5)); err != nil {
		t.Fatal(err)
	}
	if got, _ := bag.Get("Speed"); got.F != 5 {
		t.Fatal("set did not write the canonical name")
	}
}

func TestSetDynamicRejectsNativeCollision(t *testing.T) {
	r := frozen(t)
	good, _ := NewScriptType("game.A", nil, nil)
	if err := r.SetDynamic([]*TypeDescriptor{good}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"engine.Native", "engine.Old"} {
		bad, _ := NewScriptType(name, nil, nil)
		if err := r.SetDynamic([]*TypeDescriptor{bad}); err == nil {
			t.Fatalf("%s collides with the native layer but was accepted", name)
		}
	}
	if _, ok := r.ByName("game.A"); !ok {
		t.Fatal("failed SetDynamic replaced the previous layer")
	}
}

func TestSetDynamicNeedsFrozen(t *testing.T) {
	td, _ := NewScriptType("game.A", nil, nil)
	if err := NewRegistry().SetDynamic([]*TypeDescriptor{td}); err == nil {
		t.Fatal("SetDynamic on an unfrozen registry was accepted")
	}
}
