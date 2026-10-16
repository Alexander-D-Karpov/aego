package command

import (
	"testing"

	"aego/core/errs"
	"aego/platform/input"
)

func TestRegisterAndRun(t *testing.T) {
	r := New()
	ran := 0
	if err := r.Register(Desc{ID: "a", Run: func() error { ran++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(Desc{ID: "a", Run: func() error { return nil }}); !errs.Has(err, errs.EditorCommand) {
		t.Fatal("duplicate accepted")
	}
	if err := r.Run("a"); err != nil || ran != 1 {
		t.Fatalf("run err=%v ran=%d", err, ran)
	}
	enabled := false
	r.Register(Desc{ID: "b", Enabled: func() bool { return enabled }, Run: func() error { ran++; return nil }})
	if err := r.Run("b"); err == nil {
		t.Fatal("disabled command ran")
	}
}

func TestShortcutConflictsAndScopes(t *testing.T) {
	r := New()
	noop := func() error { return nil }
	sc := Key(input.KeyW, 0)
	if err := r.Register(Desc{ID: "global", Shortcut: sc, Run: noop}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(Desc{ID: "dup", Shortcut: sc, Run: noop}); err == nil {
		t.Fatal("conflict in the same scope accepted")
	}
	if err := r.Register(Desc{ID: "scene.move", Shortcut: sc, Scope: ScopeScene, Run: noop}); err != nil {
		t.Fatal(err)
	}
	if d, _ := r.Lookup(sc, ScopeScene); d.ID != "scene.move" {
		t.Fatalf("scoped lookup = %s", d.ID)
	}
	if d, _ := r.Lookup(sc, "hierarchy"); d.ID != "global" {
		t.Fatalf("fallback lookup = %s", d.ID)
	}
}

func TestDispatch(t *testing.T) {
	r := New()
	ran := ""
	r.Register(Desc{ID: EditUndo, Shortcut: Key(input.KeyZ, ModPrimary), Run: func() error { ran = EditUndo; return nil }})
	r.Register(Desc{ID: EditRedo, Shortcut: Key(input.KeyZ, ModPrimary|ModShift), Run: func() error { ran = EditRedo; return nil }})
	var in input.State
	in.BeginFrame()
	mods := input.PrimaryMod() | input.ModShift
	in.ApplyKeyDown(input.KeyZ, 0, mods, false)
	if id, ok := r.Dispatch(&in, ""); !ok || id != EditRedo || ran != EditRedo {
		t.Fatalf("dispatch = %s %v", id, ok)
	}
	in.BeginFrame()
	if _, ok := r.Dispatch(&in, ""); ok {
		t.Fatal("held key without repeat must not re-dispatch")
	}
}

func TestParseShortcut(t *testing.T) {
	sc, err := ParseShortcut("Primary+Shift+Z")
	if err != nil || sc != Key(input.KeyZ, ModPrimary|ModShift) {
		t.Fatalf("sc=%v err=%v", sc, err)
	}
	if _, err := ParseShortcut("Hyper+Z"); err == nil {
		t.Fatal("unknown modifier accepted")
	}
	if _, err := ParseShortcut("Primary+Nope"); err == nil {
		t.Fatal("unknown key accepted")
	}
}

func TestDispatchFallsBackPastDisabledScoped(t *testing.T) {
	r := New()
	ran := ""
	focused := false
	sc := Key(input.KeyD, ModPrimary)
	r.Register(Desc{ID: EditDuplicate, Shortcut: sc, Run: func() error { ran = EditDuplicate; return nil }})
	r.Register(Desc{ID: "assets.duplicate", Scope: ScopeAssets, Shortcut: sc,
		Enabled: func() bool { return focused },
		Run:     func() error { ran = "assets.duplicate"; return nil }})
	press := func() {
		var in input.State
		in.BeginFrame()
		in.ApplyKeyDown(input.KeyD, 0, input.PrimaryMod(), false)
		r.Dispatch(&in, ScopeAssets)
	}
	press()
	if ran != EditDuplicate {
		t.Fatalf("unfocused panel must fall back to the global command, ran %q", ran)
	}
	focused = true
	press()
	if ran != "assets.duplicate" {
		t.Fatalf("focused panel must win, ran %q", ran)
	}
}
