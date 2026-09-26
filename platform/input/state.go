package input

import "aego/core/mathx"

type Button uint8

const (
	MouseLeft Button = iota
	MouseRight
	MouseMiddle
	MouseX1
	MouseX2
)

const (
	ButtonCount  = 8
	TextCapacity = 64
)

var buttonNames = [...]string{"left", "right", "middle", "x1", "x2"}

func (b Button) String() string {
	if int(b) < len(buttonNames) {
		return buttonNames[b]
	}
	return "other"
}

const (
	flagDown uint8 = 1 << iota
	flagPressed
	flagReleased
	flagRepeat
)

type KeyEvent struct {
	Scan   Scancode
	Key    Keycode
	Mods   Mods
	Repeat bool
}

type MouseEvent struct {
	Pos    mathx.Vec2
	Delta  mathx.Vec2
	Wheel  mathx.Vec2
	Button Button
}

type Frame struct {
	Keys       [ScancodeCount]uint8
	Buttons    [ButtonCount]uint8
	MousePos   mathx.Vec2
	MouseDelta mathx.Vec2
	Wheel      mathx.Vec2
	Mods       Mods
	Text       [TextCapacity]byte
	TextN      uint8
}

type State struct {
	f Frame
}

func (s *State) BeginFrame() {
	for i, v := range s.f.Keys {
		s.f.Keys[i] = v & flagDown
	}
	for i, v := range s.f.Buttons {
		s.f.Buttons[i] = v & flagDown
	}
	s.f.MouseDelta = mathx.Vec2{}
	s.f.Wheel = mathx.Vec2{}
	s.f.TextN = 0
}

func (s *State) Reset() {
	for i, v := range s.f.Keys {
		if v&flagDown != 0 {
			s.f.Keys[i] = (v &^ (flagDown | flagRepeat)) | flagReleased
		}
	}
	for i, v := range s.f.Buttons {
		if v&flagDown != 0 {
			s.f.Buttons[i] = (v &^ flagDown) | flagReleased
		}
	}
	s.f.MouseDelta = mathx.Vec2{}
	s.f.Wheel = mathx.Vec2{}
	s.f.Mods = 0
}

func (s *State) ApplyKeyDown(scan Scancode, key Keycode, mods Mods, repeat bool) {
	s.f.Mods = mods
	if !scan.Valid() {
		return
	}
	if repeat {
		s.f.Keys[scan] |= flagRepeat
		return
	}
	apply(&s.f.Keys[scan], true)
}

func (s *State) KeyRepeated(k Scancode) bool {
	return int(k) < ScancodeCount && s.f.Keys[k]&(flagPressed|flagRepeat) != 0
}

func (s *State) ApplyKeyUp(scan Scancode, key Keycode, mods Mods) {
	s.f.Mods = mods
	if !scan.Valid() {
		return
	}
	apply(&s.f.Keys[scan], false)
}

func (s *State) ApplyMouseMove(pos, delta mathx.Vec2) {
	s.f.MousePos = pos
	s.f.MouseDelta = s.f.MouseDelta.Add(delta)
}

func (s *State) ApplyMouseDown(pos mathx.Vec2, b Button) {
	s.f.MousePos = pos
	if int(b) >= ButtonCount {
		return
	}
	apply(&s.f.Buttons[b], true)
}

func (s *State) ApplyMouseUp(pos mathx.Vec2, b Button) {
	s.f.MousePos = pos
	if int(b) >= ButtonCount {
		return
	}
	apply(&s.f.Buttons[b], false)
}

func (s *State) ApplyWheel(pos, wheel mathx.Vec2) {
	s.f.MousePos = pos
	s.f.Wheel = s.f.Wheel.Add(wheel)
}

func (s *State) ApplyText(text []byte) {
	if s.f.TextN >= TextCapacity {
		return
	}
	n := copy(s.f.Text[s.f.TextN:], text)
	s.f.TextN += uint8(n)
}

func apply(flags *uint8, down bool) {
	if down {
		if *flags&flagDown == 0 {
			*flags |= flagDown | flagPressed
		}
		return
	}
	if *flags&flagDown != 0 {
		*flags = (*flags &^ flagDown) | flagReleased
	}
}

func (s *State) KeyDown(k Scancode) bool {
	return int(k) < ScancodeCount && s.f.Keys[k]&flagDown != 0
}

func (s *State) KeyPressed(k Scancode) bool {
	return int(k) < ScancodeCount && s.f.Keys[k]&flagPressed != 0
}

func (s *State) KeyReleased(k Scancode) bool {
	return int(k) < ScancodeCount && s.f.Keys[k]&flagReleased != 0
}

func (s *State) Mods() Mods {
	return s.f.Mods
}

func (s *State) MousePos() mathx.Vec2 {
	return s.f.MousePos
}

func (s *State) MouseDelta() mathx.Vec2 {
	return s.f.MouseDelta
}

func (s *State) Wheel() mathx.Vec2 {
	return s.f.Wheel
}

func (s *State) ButtonDown(b Button) bool {
	return int(b) < ButtonCount && s.f.Buttons[b]&flagDown != 0
}

func (s *State) ButtonPressed(b Button) bool {
	return int(b) < ButtonCount && s.f.Buttons[b]&flagPressed != 0
}

func (s *State) ButtonReleased(b Button) bool {
	return int(b) < ButtonCount && s.f.Buttons[b]&flagReleased != 0
}

func (s *State) Text() []byte {
	return s.f.Text[:s.f.TextN]
}

func (s *State) EachDown(fn func(Scancode)) {
	for i, v := range s.f.Keys {
		if v&flagDown != 0 {
			fn(Scancode(i))
		}
	}
}

func (s *State) DownCount() int {
	n := 0
	for _, v := range s.f.Keys {
		if v&flagDown != 0 {
			n++
		}
	}
	return n
}

func (s *State) Snapshot() Frame {
	return s.f
}

func (s *State) Restore(f Frame) {
	s.f = f
}
