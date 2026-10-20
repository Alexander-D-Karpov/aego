package web

import (
	"testing"

	"aego/platform"
	"aego/platform/input"
)

func TestScancodes(t *testing.T) {
	cases := map[string]input.Scancode{
		"KeyA": input.KeyA, "KeyZ": input.KeyZ, "Digit1": input.Key1, "Digit0": input.Key0, "F12": input.KeyF12,
		"ArrowUp": input.KeyUp, "Numpad0": input.KeyKP0, "Numpad9": input.KeyKP9, "ShiftLeft": input.KeyLShift,
		"MetaRight": input.KeyRGUI, "Space": input.KeySpace, "Quote": input.KeyApostrophe,
	}
	for code, want := range cases {
		if got := scancodeOf(code); got != want {
			t.Errorf("%s = %v, want %v", code, got, want)
		}
	}
	if scancodeOf("Lang1") != input.KeyUnknown {
		t.Error("an unknown code must map to KeyUnknown")
	}
}

func TestKeycodes(t *testing.T) {
	if k := keycodeOf("A", input.KeyA); k != 'a' {
		t.Fatalf("A = %v", k)
	}
	if k := keycodeOf(" ", input.KeySpace); k != ' ' {
		t.Fatalf("space = %v", k)
	}
	if k := keycodeOf("ArrowUp", input.KeyUp); !k.IsScancodeBased() {
		t.Fatalf("ArrowUp = %v", k)
	}
}

func TestText(t *testing.T) {
	if s, ok := textOf("ж", 0); !ok || s != "ж" {
		t.Fatal("a letter is text")
	}
	if _, ok := textOf("c", input.ModCtrl); ok {
		t.Fatal("ctrl+c is a shortcut")
	}
	if _, ok := textOf("@", input.ModCtrl|input.ModAlt); !ok {
		t.Fatal("AltGr text must pass")
	}
	if _, ok := textOf("Enter", 0); ok {
		t.Fatal("Enter is not text")
	}
}

func TestWheel(t *testing.T) {
	if v, precise := wheelOf(0, 100, deltaPixel); v.Y != -1 || precise {
		t.Fatalf("notch = %v %v", v, precise)
	}
	if v, precise := wheelOf(0, 4, deltaPixel); !precise || v.Y >= 0 {
		t.Fatalf("trackpad = %v %v", v, precise)
	}
	if v, _ := wheelOf(30, 0, deltaPixel); v.X <= 0 {
		t.Fatalf("horizontal = %v", v)
	}
	if v, precise := wheelOf(0, 1, 1); v.Y != -1 || precise {
		t.Fatalf("lines = %v", v)
	}
	if v, _ := wheelOf(0, 1, deltaPage); v.Y != -linesPerPage {
		t.Fatalf("page = %v", v)
	}
}

func TestBrowserKeys(t *testing.T) {
	for _, c := range []struct {
		code string
		mods input.Mods
		want bool
	}{{"F5", 0, true}, {"F12", 0, true}, {"KeyR", input.ModCtrl, true}, {"KeyC", input.ModSuper, true}, {"Space", 0, false}, {"KeyW", 0, false}, {"Escape", 0, false}} {
		if got := browserKey(c.code, c.mods); got != c.want {
			t.Errorf("%s %v = %v", c.code, c.mods, got)
		}
	}
}

func kinds(evs []platform.Event) []platform.EventKind {
	out := make([]platform.EventKind, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

func TestPadDiff(t *testing.T) {
	var s padSlot
	f := padFrame{connected: true, name: "pad"}
	f.buttons[input.PadA] = true
	f.axes[input.PadLeftX] = 0.5
	evs := s.diff(1, &f, nil)
	want := []platform.EventKind{platform.EvPadAdded, platform.EvPadButtonDown, platform.EvPadAxis}
	if got := kinds(evs); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("connect = %v", got)
	}
	if evs[0].Pad.Name != "pad" || evs[1].Pad.Button != input.PadA || evs[2].Pad.Value != 0.5 {
		t.Fatalf("payload = %+v", evs)
	}
	if evs := s.diff(1, &f, nil); len(evs) != 0 {
		t.Fatalf("unchanged pad emitted %v", kinds(evs))
	}
	f.buttons[input.PadA] = false
	f.axes[input.PadLeftX] = 0
	if got := kinds(s.diff(1, &f, nil)); len(got) != 2 || got[0] != platform.EvPadButtonUp || got[1] != platform.EvPadAxis {
		t.Fatalf("release = %v", got)
	}
	f.connected = false
	if got := kinds(s.diff(1, &f, nil)); len(got) != 1 || got[0] != platform.EvPadRemoved {
		t.Fatalf("disconnect = %v", got)
	}
}

func TestCursorNames(t *testing.T) {
	for s := platform.CursorDefault; s <= platform.CursorCrosshair; s++ {
		if cursorName(s) == "" {
			t.Errorf("cursor %d has no css name", s)
		}
	}
}

func TestMods(t *testing.T) {
	if m := modsOf(true, false, true, false, true, false); m != input.ModShift|input.ModAlt|input.ModCaps {
		t.Fatalf("mods = %v", m)
	}
	if modsOf(false, false, false, true, false, true) != input.ModSuper|input.ModNum {
		t.Fatal("meta and num lock")
	}
}

func TestPointerButtons(t *testing.T) {
	for b, want := range []input.Button{input.MouseLeft, input.MouseMiddle, input.MouseRight, input.MouseX1, input.MouseX2} {
		if got, ok := pointerButton(b); !ok || got != want {
			t.Errorf("button %d = %v", b, got)
		}
	}
	if _, ok := pointerButton(5); ok {
		t.Fatal("button 5 accepted")
	}
}

func TestStandardMapping(t *testing.T) {
	seen := map[input.PadButton]bool{}
	for _, m := range standardPad {
		if seen[m.button] || m.index == padLT || m.index == padRT {
			t.Fatalf("index %d maps %v twice or shadows a trigger", m.index, m.button)
		}
		seen[m.button] = true
	}
	if len(seen) != int(input.PadButtonCount) {
		t.Fatalf("%d of %d pad buttons mapped", len(seen), input.PadButtonCount)
	}
	axes := map[input.PadAxis]bool{input.PadLT: true, input.PadRT: true}
	for _, a := range standardAxes {
		axes[a] = true
	}
	if len(axes) != int(input.PadAxisCount) {
		t.Fatalf("%d of %d pad axes mapped", len(axes), input.PadAxisCount)
	}
}

func TestSmoothWheelScrollsPagePixels(t *testing.T) {
	v, precise := wheelOf(0, 53, deltaPixel)
	if !precise || v.Y > -5.29 || v.Y < -5.31 {
		t.Fatalf("got %v %v, want -5.3 precise units (53 px)", v, precise)
	}
}
