package platform

import "aego/platform/input"

type EventKind uint8

const (
	EvQuit EventKind = iota
	EvWindowResized
	EvWindowFocusGained
	EvWindowFocusLost
	EvWindowMinimized
	EvWindowRestored
	EvKeyDown
	EvKeyUp
	EvTextInput
	EvMouseMove
	EvMouseDown
	EvMouseUp
	EvMouseWheel
	EvTextEditing
	EvDropBegin
	EvDropFile
	EvDropText
	EvDropComplete
)

var eventKindNames = [...]string{
	"Quit", "WindowResized", "WindowFocusGained", "WindowFocusLost",
	"WindowMinimized", "WindowRestored", "KeyDown", "KeyUp", "TextInput",
	"MouseMove", "MouseDown", "MouseUp", "MouseWheel",
	"TextEditing", "DropBegin", "DropFile", "DropText", "DropComplete",
}

func (k EventKind) String() string {
	if int(k) < len(eventKindNames) {
		return eventKindNames[k]
	}
	return "Unknown"
}

type Event struct {
	Kind      EventKind
	Key       input.KeyEvent
	Mouse     input.MouseEvent
	Text      [32]byte
	TextN     uint8
	Str       string
	EditStart int32
	EditLen   int32
	Window    WindowMetrics
}

func (e *Event) TextBytes() []byte {
	return e.Text[:e.TextN]
}

func ApplyInput(s *input.State, ev Event) {
	switch ev.Kind {
	case EvKeyDown:
		s.ApplyKeyDown(ev.Key.Scan, ev.Key.Key, ev.Key.Mods, ev.Key.Repeat)
	case EvKeyUp:
		s.ApplyKeyUp(ev.Key.Scan, ev.Key.Key, ev.Key.Mods)
	case EvMouseMove:
		s.ApplyMouseMove(ev.Mouse.Pos, ev.Mouse.Delta)
	case EvMouseDown:
		s.ApplyMouseDown(ev.Mouse.Pos, ev.Mouse.Button)
	case EvMouseUp:
		s.ApplyMouseUp(ev.Mouse.Pos, ev.Mouse.Button)
	case EvMouseWheel:
		s.ApplyWheel(ev.Mouse.Pos, ev.Mouse.Wheel)
	case EvTextInput:
		s.ApplyText(ev.Text[:ev.TextN])
	case EvWindowFocusLost:
		s.Reset()
	}
}
