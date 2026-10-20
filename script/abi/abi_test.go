package abi

import "testing"

func TestTypeRefRoundTrip(t *testing.T) {
	for _, r := range []TypeRef{
		T(Int), T(Transform3D), SliceOf(SliceOf(T(String))), AssetOf("texture"),
		ComponentOf("engine.SpriteRenderer"), EnumOf("aego/input.Key"), StructOf("game.Stats"),
		T(Coroutine), T(Connection), ScriptRefOf("game.enemy.EnemyAI"), SliceOf(T(Coroutine)),
	} {
		got, ok := Parse(r.String())
		if !ok || !got.Equal(r) {
			t.Fatalf("%s -> %v %v", r, got, ok)
		}
	}
	for _, bad := range []string{"", "[]void", "enum:", "Vec9", "AssetRef<texture", "ScriptRef<>"} {
		if _, ok := Parse(bad); ok {
			t.Fatalf("%q parsed", bad)
		}
	}
	if Sig(T(Void), T(Coroutine)).Validate() == nil {
		t.Fatal("Coroutine must not cross the host boundary")
	}
}

func mustAdd(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestHashIsStableAndSensitive(t *testing.T) {
	build := func(extra bool) *Table {
		tb := NewTable()
		mustAdd(t, tb.AddFunc("aego/log", "Info", Sig(T(Void), T(String))))
		mustAdd(t, tb.AddFunc("aego/math", "Abs", Sig(T(Float), T(Float))))
		if extra {
			mustAdd(t, tb.AddFunc("aego/math", "Sqrt", Sig(T(Float), T(Float))))
		}
		tb.Freeze()
		return tb
	}
	if a, b := build(false).Hash(), build(false).Hash(); a != b {
		t.Fatal("hash is not deterministic")
	}
	if build(false).Hash() == build(true).Hash() {
		t.Fatal("hash ignores a new function")
	}
	if w := Sig(T(Transform2D), T(Int), T(Int), T(Int), T(Int)).Window(); w != 4 {
		t.Fatalf("window %d", w)
	}
}

func TestComponentIndexNotInHash(t *testing.T) {
	// Registration order changes Index but not identity; the hash must ignore it.
	a := NewTable()
	mustAdd(t, a.AddComponent(&Component{Name: "engine.A"}))
	mustAdd(t, a.AddComponent(&Component{Name: "engine.B"}))
	a.Freeze()
	b := NewTable()
	mustAdd(t, b.AddComponent(&Component{Name: "engine.B"}))
	mustAdd(t, b.AddComponent(&Component{Name: "engine.A"}))
	b.Freeze()
	if a.Hash() != b.Hash() {
		t.Fatal("component registration order must not change the hash")
	}
}

func TestRegistrationGuards(t *testing.T) {
	tb := NewTable()
	mustAdd(t, tb.AddFunc("m", "Struct", Sig(StructOf("m.Hit"), StructOf("m.Hit"))))
	mustAdd(t, tb.AddFunc("m", "Fill", Sig(T(Int), SliceOf(StructOf("m.Hit")))))
	if tb.AddFunc("m", "BadSlice", Sig(SliceOf(T(Int)))) == nil {
		t.Error("slice result must be rejected")
	}
	if tb.AddFunc("m", "BadElem", Sig(T(Void), SliceOf(T(String)))) == nil {
		t.Error("slice of heap values must be rejected")
	}
	if tb.AddStruct(&StructDef{Module: "m", Name: "Bad", Slots: 1, Fields: []StructField{{Name: "S", Type: T(String)}}}) == nil {
		t.Error("struct with a heap field must be rejected")
	}
	if tb.AddEvent(EventDef{Name: "hit"}) == nil {
		t.Error("unexported event must be rejected")
	}
	// Strings are allowed.
	mustAdd(t, tb.AddFunc("m", "Ok", Sig(T(String), T(String))))
	// Duplicates are rejected.
	if tb.AddFunc("m", "Ok", Sig(T(String), T(String))) == nil {
		t.Error("duplicate func must be rejected")
	}
	// After Freeze the table is sealed.
	tb.Freeze()
	if tb.AddFunc("m", "Late", Sig(T(Void))) == nil {
		t.Error("registration after Freeze must be rejected")
	}
}

func TestHashPanicsWhenUnfrozen(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Hash on an unfrozen table must panic")
		}
	}()
	NewTable().Hash()
}

func TestEventStringParam(t *testing.T) {
	tab := NewTable()
	if err := tab.AddEvent(EventDef{Name: "OnNamed", Params: []TypeRef{T(String)}}); err != nil {
		t.Fatal(err)
	}
	if err := tab.AddStruct(&StructDef{Module: "m", Name: "S", Slots: 1, Fields: []StructField{{Name: "s", Type: T(String)}}}); err == nil {
		t.Fatal("a host struct accepted a string field")
	}
	if err := tab.AddEvent(EventDef{Name: "OnList", Params: []TypeRef{SliceOf(T(Int))}}); err == nil {
		t.Fatal("an event accepted a slice")
	}
}
