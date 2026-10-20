package web

import (
	"strconv"
	"unicode"
	"unicode/utf8"

	"aego/core/mathx"
	"aego/platform"
	"aego/platform/input"
)

const (
	pixelsPerLine = 100
	linesPerPage  = 3
	scancodeBit   = 1 << 30
	axisEpsilon   = 1.0 / 512
	padLT         = 6
	padRT         = 7
	deltaPixel    = 0
	deltaPage     = 2
)

var scancodes = buildScancodes()

func buildScancodes() map[string]input.Scancode {
	m := map[string]input.Scancode{
		"Enter": input.KeyEnter, "Escape": input.KeyEscape, "Backspace": input.KeyBackspace, "Tab": input.KeyTab,
		"Space": input.KeySpace, "Minus": input.KeyMinus, "Equal": input.KeyEquals,
		"BracketLeft": input.KeyLeftBracket, "BracketRight": input.KeyRightBracket, "Backslash": input.KeyBackslash,
		"Semicolon": input.KeySemicolon, "Quote": input.KeyApostrophe, "Backquote": input.KeyGrave,
		"Comma": input.KeyComma, "Period": input.KeyPeriod, "Slash": input.KeySlash, "CapsLock": input.KeyCapsLock,
		"PrintScreen": input.KeyPrintScreen, "ScrollLock": input.KeyScrollLock, "Pause": input.KeyPause,
		"Insert": input.KeyInsert, "Home": input.KeyHome, "PageUp": input.KeyPageUp, "Delete": input.KeyDelete,
		"End": input.KeyEnd, "PageDown": input.KeyPageDown,
		"ArrowRight": input.KeyRight, "ArrowLeft": input.KeyLeft, "ArrowDown": input.KeyDown, "ArrowUp": input.KeyUp,
		"NumLock": input.KeyNumLock, "NumpadDivide": input.KeyKPDivide, "NumpadMultiply": input.KeyKPMultiply,
		"NumpadSubtract": input.KeyKPMinus, "NumpadAdd": input.KeyKPPlus, "NumpadEnter": input.KeyKPEnter,
		"NumpadDecimal": input.KeyKPPeriod,
		"ControlLeft":   input.KeyLCtrl, "ShiftLeft": input.KeyLShift, "AltLeft": input.KeyLAlt, "MetaLeft": input.KeyLGUI,
		"ControlRight": input.KeyRCtrl, "ShiftRight": input.KeyRShift, "AltRight": input.KeyRAlt, "MetaRight": input.KeyRGUI,
		"Digit0": input.Key0, "Numpad0": input.KeyKP0,
	}
	for i := 0; i < 26; i++ {
		m["Key"+string(rune('A'+i))] = input.KeyA + input.Scancode(i)
	}
	for i := 1; i <= 9; i++ {
		n := strconv.Itoa(i)
		m["Digit"+n] = input.Key1 + input.Scancode(i-1)
		m["Numpad"+n] = input.KeyKP1 + input.Scancode(i-1)
	}
	for i := 1; i <= 12; i++ {
		m["F"+strconv.Itoa(i)] = input.KeyF1 + input.Scancode(i-1)
	}
	return m
}

func scancodeOf(code string) input.Scancode {
	return scancodes[code]
}

func printable(s string) (rune, bool) {
	r, n := utf8.DecodeRuneInString(s)
	return r, n > 0 && n == len(s) && unicode.IsPrint(r)
}

func keycodeOf(key string, scan input.Scancode) input.Keycode {
	if r, ok := printable(key); ok {
		return input.Keycode(unicode.ToLower(r))
	}
	return input.Keycode(scancodeBit | uint32(scan))
}

func shortcut(mods input.Mods) bool {
	return mods&input.ModSuper != 0 || (mods&input.ModCtrl != 0 && mods&input.ModAlt == 0)
}

func textOf(key string, mods input.Mods) (string, bool) {
	if shortcut(mods) {
		return "", false
	}
	if _, ok := printable(key); !ok {
		return "", false
	}
	return key, true
}

func modsOf(shift, ctrl, alt, meta, caps, num bool) input.Mods {
	var m input.Mods
	for _, f := range [...]struct {
		on  bool
		bit input.Mods
	}{{shift, input.ModShift}, {ctrl, input.ModCtrl}, {alt, input.ModAlt}, {meta, input.ModSuper}, {caps, input.ModCaps}, {num, input.ModNum}} {
		if f.on {
			m |= f.bit
		}
	}
	return m
}

func browserKey(code string, mods input.Mods) bool {
	if mods&(input.ModCtrl|input.ModSuper) != 0 {
		return true
	}
	switch code {
	case "F5", "F11", "F12":
		return true
	}
	return false
}

func fractional(v float32) bool {
	return v != float32(int32(v))
}

func wheelOf(dx, dy float64, mode int) (mathx.Vec2, bool) {
	sx, sy := dx, -dy
	switch mode {
	case deltaPixel:
		sx, sy = sx/pixelsPerLine, sy/pixelsPerLine
	case deltaPage:
		sx, sy = sx*linesPerPage, sy*linesPerPage
	}
	v := mathx.Vec2{X: float32(sx), Y: float32(sy)}
	return v, fractional(v.X) || fractional(v.Y)
}

func pointerButton(b int) (input.Button, bool) {
	switch b {
	case 0:
		return input.MouseLeft, true
	case 1:
		return input.MouseMiddle, true
	case 2:
		return input.MouseRight, true
	case 3:
		return input.MouseX1, true
	case 4:
		return input.MouseX2, true
	}
	return 0, false
}

var cursorNames = [...]string{
	platform.CursorDefault:    "default",
	platform.CursorText:       "text",
	platform.CursorPointer:    "pointer",
	platform.CursorMove:       "move",
	platform.CursorResizeEW:   "ew-resize",
	platform.CursorResizeNS:   "ns-resize",
	platform.CursorResizeNWSE: "nwse-resize",
	platform.CursorResizeNESW: "nesw-resize",
	platform.CursorNotAllowed: "not-allowed",
	platform.CursorCrosshair:  "crosshair",
}

func cursorName(s platform.CursorShape) string {
	if int(s) < len(cursorNames) {
		return cursorNames[s]
	}
	return cursorNames[platform.CursorDefault]
}

var standardPad = [...]struct {
	index  int
	button input.PadButton
}{
	{0, input.PadA}, {1, input.PadB}, {2, input.PadX}, {3, input.PadY}, {4, input.PadLB}, {5, input.PadRB},
	{8, input.PadBack}, {9, input.PadStart}, {10, input.PadL3}, {11, input.PadR3},
	{12, input.PadUp}, {13, input.PadDown}, {14, input.PadLeft}, {15, input.PadRight}, {16, input.PadGuide},
}

var standardAxes = [...]input.PadAxis{input.PadLeftX, input.PadLeftY, input.PadRightX, input.PadRightY}

type padFrame struct {
	connected bool
	name      string
	buttons   [input.PadButtonCount]bool
	axes      [input.PadAxisCount]float32
}

type padSlot struct {
	on      bool
	buttons [input.PadButtonCount]bool
	axes    [input.PadAxisCount]float32
}

func (s *padSlot) diff(id uint32, f *padFrame, out []platform.Event) []platform.Event {
	switch {
	case !f.connected && s.on:
		*s = padSlot{}
		return append(out, platform.Event{Kind: platform.EvPadRemoved, Pad: input.PadEvent{ID: id}})
	case !f.connected:
		return out
	case !s.on:
		s.on = true
		out = append(out, platform.Event{Kind: platform.EvPadAdded, Pad: input.PadEvent{ID: id, Name: f.name}})
	}
	for b := range f.buttons {
		if f.buttons[b] == s.buttons[b] {
			continue
		}
		s.buttons[b] = f.buttons[b]
		kind := platform.EvPadButtonUp
		if f.buttons[b] {
			kind = platform.EvPadButtonDown
		}
		out = append(out, platform.Event{Kind: kind, Pad: input.PadEvent{ID: id, Button: input.PadButton(b)}})
	}
	for a := range f.axes {
		v, old := f.axes[a], s.axes[a]
		if mathx.Abs(v-old) < axisEpsilon && (v != 0 || old == 0) {
			continue
		}
		s.axes[a] = v
		out = append(out, platform.Event{Kind: platform.EvPadAxis, Pad: input.PadEvent{ID: id, Axis: input.PadAxis(a), Value: v}})
	}
	return out
}
