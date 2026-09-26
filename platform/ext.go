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
