package command

import (
	"errors"
	"testing"

	"aego/platform/input"
)

func TestRunFromSearchUsesOptionalAction(t *testing.T) {
	r := New()
	var normal, search int
	failure := errors.New("search failed")
	var reported error
	r.OnError = func(id string, err error) { reported = err }
	if err := r.Register(Desc{ID: "window.example", Run: func() error {
		normal++
		return nil
	}, SearchRun: func() error {
		search++
		return failure
	}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Run("window.example"); err != nil || normal != 1 || search != 0 {
		t.Fatalf("normal action: %v, normal %d, search %d", err, normal, search)
	}
	if err := r.RunFromSearch("window.example"); !errors.Is(err, failure) || !errors.Is(reported, failure) || search != 1 {
		t.Fatalf("search action: %v, reported %v, ran %d", err, reported, search)
	}
	if err := r.Register(Desc{ID: "ordinary", Run: func() error { normal++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := r.RunFromSearch("ordinary"); err != nil || normal != 2 {
		t.Fatalf("ordinary action from search: %v, ran %d", err, normal)
	}
	if err := r.Register(Desc{ID: "disabled", Enabled: func() bool { return false }, Run: func() error { return nil }, SearchRun: func() error { search++; return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := r.RunFromSearch("disabled"); err == nil || search != 1 {
		t.Fatal("search must not run a disabled command")
	}
}

func TestAltShortcutDispatches(t *testing.T) {
	r := New()
	ran := 0
	if err := r.Register(Desc{ID: "a", Shortcut: Key(input.KeyF6, 0), Alt: []Shortcut{Key(input.KeyP, ModPrimary|ModShift)}, Run: func() error {
		ran++
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	in := &input.State{}
	in.ApplyKeyDown(input.KeyP, 0, input.PrimaryMod()|input.ModShift, false)
	if id, ok := r.Dispatch(in, ""); !ok || id != "a" || ran != 1 {
		t.Fatalf("dispatch %q %v ran %d", id, ok, ran)
	}
	if err := r.Register(Desc{ID: "b", Shortcut: Key(input.KeyP, ModPrimary|ModShift), Run: func() error { return nil }}); err == nil {
		t.Fatal("an alternative shortcut must conflict like the primary one")
	}
}

func TestPlayKeysLeavePaletteFree(t *testing.T) {
	sc := DefaultShortcuts()
	palette := Key(input.KeyP, ModPrimary|ModShift)
	for id, s := range sc {
		if s == palette {
			t.Fatalf("%s still uses Ctrl+Shift+P", id)
		}
	}
	if sc[PlayPause] != Key(input.KeyF6, 0) || sc[PlayStepFixed] != Key(input.KeyF7, 0) || sc[PlayStep] != Key(input.KeyF8, 0) {
		t.Fatal("play keys are not F6, F7, F8")
	}
}

func TestReleaseInputIsShiftEscape(t *testing.T) {
	r := New()
	if err := r.Register(Desc{ID: PlayReleaseInput, Name: "Release", Shortcut: Key(input.KeyEscape, ModShift), Run: func() error { return nil }}); err != nil {
		t.Fatal(err)
	}
	var in input.State
	in.ApplyKeyDown(input.KeyEscape, 0, 0, false)
	if r.Pressed(PlayReleaseInput, &in) {
		t.Fatal("plain Escape matched")
	}
	in.BeginFrame()
	in.ApplyKeyUp(input.KeyEscape, 0, 0)
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEscape, 0, input.ModShift|input.ModNum, false)
	if !r.Pressed(PlayReleaseInput, &in) {
		t.Fatal("Shift+Esc did not match with NumLock on")
	}
}
