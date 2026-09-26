package input

import (
	"testing"

	"aego/core/mathx"
)

func TestPressHoldRelease(t *testing.T) {
	var s State
	s.BeginFrame()
	s.ApplyKeyDown(KeyA, 0, 0, false)
	if !s.KeyDown(KeyA) || !s.KeyPressed(KeyA) || s.KeyReleased(KeyA) {
		t.Fatal("press frame wrong")
	}
	s.BeginFrame()
	s.ApplyKeyDown(KeyA, 0, 0, true)
	if !s.KeyDown(KeyA) || s.KeyPressed(KeyA) {
		t.Fatal("repeat must not produce a new press")
	}
	s.BeginFrame()
	s.ApplyKeyUp(KeyA, 0, 0)
	if s.KeyDown(KeyA) || !s.KeyReleased(KeyA) {
		t.Fatal("release frame wrong")
	}
	s.BeginFrame()
	if s.KeyReleased(KeyA) || s.KeyPressed(KeyA) {
		t.Fatal("edge flags leaked into the next frame")
	}
}

func TestTapWithinFrame(t *testing.T) {
	var s State
	s.BeginFrame()
	s.ApplyKeyDown(KeySpace, 0, 0, false)
	s.ApplyKeyUp(KeySpace, 0, 0)
	if s.KeyDown(KeySpace) || !s.KeyPressed(KeySpace) || !s.KeyReleased(KeySpace) {
		t.Fatal("tap must report both pressed and released")
	}
}

func TestResetOnFocusLoss(t *testing.T) {
	var s State
	s.BeginFrame()
	s.ApplyKeyDown(KeyW, 0, ModShift|ModCtrl, false)
	s.ApplyMouseDown(mathx.Vec2{}, MouseLeft)
	s.BeginFrame()
	s.Reset()
	if s.KeyDown(KeyW) || s.ButtonDown(MouseLeft) {
		t.Fatal("keys stuck after reset")
	}
	if !s.KeyReleased(KeyW) || !s.ButtonReleased(MouseLeft) {
		t.Fatal("reset must emit a release edge")
	}
	if s.Mods() != 0 || s.DownCount() != 0 {
		t.Fatal("reset left residual state")
	}
}

func TestResetKeepsEarlierEdges(t *testing.T) {
	var s State
	s.BeginFrame()
	s.ApplyKeyDown(KeyQ, 0, 0, false)
	s.ApplyKeyUp(KeyQ, 0, 0)
	s.Reset()
	if !s.KeyPressed(KeyQ) || !s.KeyReleased(KeyQ) {
		t.Fatal("reset dropped edges of a key that was not held")
	}
}

func TestMouseAccumulation(t *testing.T) {
	var s State
	s.BeginFrame()
	s.ApplyMouseMove(mathx.Vec2{X: 10, Y: 5}, mathx.Vec2{X: 1})
	s.ApplyMouseMove(mathx.Vec2{X: 12, Y: 5}, mathx.Vec2{X: 2})
	s.ApplyWheel(mathx.Vec2{X: 12, Y: 5}, mathx.Vec2{Y: 1})
	s.ApplyWheel(mathx.Vec2{X: 12, Y: 5}, mathx.Vec2{Y: 2})
	if s.MouseDelta().X != 3 || s.MousePos().X != 12 || s.Wheel().Y != 3 {
		t.Fatalf("delta=%v pos=%v wheel=%v", s.MouseDelta(), s.MousePos(), s.Wheel())
	}
	s.BeginFrame()
	if s.MouseDelta() != (mathx.Vec2{}) || s.Wheel() != (mathx.Vec2{}) {
		t.Fatal("per-frame accumulators not cleared")
	}
	if s.MousePos().X != 12 {
		t.Fatal("absolute position must persist")
	}
}

func TestText(t *testing.T) {
	var s State
	s.BeginFrame()
	s.ApplyText([]byte("hi"))
	s.ApplyText([]byte("!"))
	if string(s.Text()) != "hi!" {
		t.Fatalf("text = %q", s.Text())
	}
	for i := 0; i < TextCapacity; i++ {
		s.ApplyText([]byte("x"))
	}
	if len(s.Text()) != TextCapacity {
		t.Fatalf("text capacity overflow: %d", len(s.Text()))
	}
	s.BeginFrame()
	if len(s.Text()) != 0 {
		t.Fatal("text not cleared")
	}
}

func TestOutOfRangeInput(t *testing.T) {
	var s State
	s.ApplyKeyDown(Scancode(ScancodeCount+10), 0, ModAlt, false)
	s.ApplyMouseDown(mathx.Vec2{}, Button(ButtonCount+3))
	if s.DownCount() != 0 {
		t.Fatal("out of range scancode was recorded")
	}
	if s.Mods() != ModAlt {
		t.Fatal("mods must be taken even from unknown keys")
	}
}

func TestEachDownAndSnapshot(t *testing.T) {
	var s State
	s.ApplyKeyDown(KeyW, 0, 0, false)
	s.ApplyKeyDown(KeyD, 0, 0, false)
	var seen []Scancode
	s.EachDown(func(k Scancode) { seen = append(seen, k) })
	if len(seen) != 2 || seen[0] != KeyW && seen[1] != KeyW {
		t.Fatalf("each down = %v", seen)
	}
	var other State
	other.Restore(s.Snapshot())
	if !other.KeyDown(KeyW) || !other.KeyDown(KeyD) {
		t.Fatal("restore lost state")
	}
	other.Reset()
	if s.DownCount() != 2 {
		t.Fatal("snapshot must be a copy, not a reference")
	}
}

func TestNames(t *testing.T) {
	keys := map[Scancode]string{
		KeyA: "A", KeyZ: "Z", Key1: "1", Key0: "0",
		KeyF1: "F1", KeyF12: "F12", KeyKP1: "KP1", KeyKP0: "KP0",
		KeyEscape: "Escape", KeyLGUI: "LGUI", Scancode(300): "Unknown",
	}
	for k, want := range keys {
		if got := k.String(); got != want {
			t.Fatalf("scancode %d: %q != %q", k, got, want)
		}
	}
	if MouseLeft.String() != "left" || Button(6).String() != "other" {
		t.Fatal("button names wrong")
	}
	if (ModShift|ModCtrl).String() != "shift+ctrl" || Mods(0).String() != "none" {
		t.Fatalf("mods string = %q", (ModShift | ModCtrl).String())
	}
	if r, ok := Keycode('a').Rune(); !ok || r != 'a' {
		t.Fatal("keycode rune wrong")
	}
	if _, ok := (keycodeScancodeBit | Keycode(KeyF1)).Rune(); ok {
		t.Fatal("scancode-based keycode must not decode to a rune")
	}
}
