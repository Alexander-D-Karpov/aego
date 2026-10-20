package command

import (
	"testing"

	"aego/platform/input"
)

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
