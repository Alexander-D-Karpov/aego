package platform

import "aego/core/mathx"

type CursorShape uint8

const (
	CursorDefault CursorShape = iota
	CursorText
	CursorPointer
	CursorMove
	CursorResizeEW
	CursorResizeNS
	CursorResizeNWSE
	CursorResizeNESW
	CursorNotAllowed
	CursorCrosshair
)

type TextInputer interface {
	StartTextInput()
	StopTextInput()
	SetTextInputArea(r mathx.Rect, cursor int)
}

type Clipboard interface {
	ClipboardText() string
	SetClipboardText(s string)
	SetClipboardData(mime string, data []byte, text string) bool
	ClipboardData(mime string) ([]byte, bool)
}

type Cursorer interface {
	SetCursor(CursorShape)
}

type MouseCapturer interface {
	CaptureMouse(on bool)
}

type WindowMode uint8

const (
	Windowed WindowMode = iota
	Borderless
	Fullscreen
)

type DisplayMode struct {
	W, H int
	Hz   float32
}

type Display struct {
	ID      uint32
	Name    string
	Current DisplayMode
	Modes   []DisplayMode
}

func (d *Display) Has(m DisplayMode) bool {
	for _, x := range d.Modes {
		if x.W == m.W && x.H == m.H {
			return true
		}
	}
	return false
}

type Displayer interface {
	Displays() []Display
	WindowDisplay() int
	WindowMode() WindowMode
	SetWindowMode(mode WindowMode, display int, size DisplayMode) error
}

type Driver interface {
	Drive(step func() bool)
}

type SystemTheme uint8

const (
	ThemeUnknown SystemTheme = iota
	ThemeLight
	ThemeDark
)

// SystemThemer reports whether the OS asks for a light or a dark look (D345).
type SystemThemer interface {
	SystemTheme() SystemTheme
}

// Localer reports the languages the user prefers, most wanted first, as "ru-RU" or "en" (D348).
type Localer interface {
	PreferredLocales() []string
}
