package input

import (
	"runtime"
	"strings"
)

type Scancode uint16

const ScancodeCount = 512

const KeyUnknown Scancode = 0

const (
	KeyA Scancode = iota + 4
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	Key0
	KeyEnter
	KeyEscape
	KeyBackspace
	KeyTab
	KeySpace
	KeyMinus
	KeyEquals
	KeyLeftBracket
	KeyRightBracket
	KeyBackslash
	KeyNonUSHash
	KeySemicolon
	KeyApostrophe
	KeyGrave
	KeyComma
	KeyPeriod
	KeySlash
	KeyCapsLock
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyPrintScreen
	KeyScrollLock
	KeyPause
	KeyInsert
	KeyHome
	KeyPageUp
	KeyDelete
	KeyEnd
	KeyPageDown
	KeyRight
	KeyLeft
	KeyDown
	KeyUp
	KeyNumLock
	KeyKPDivide
	KeyKPMultiply
	KeyKPMinus
	KeyKPPlus
	KeyKPEnter
	KeyKP1
	KeyKP2
	KeyKP3
	KeyKP4
	KeyKP5
	KeyKP6
	KeyKP7
	KeyKP8
	KeyKP9
	KeyKP0
	KeyKPPeriod
)

const (
	KeyLCtrl Scancode = iota + 224
	KeyLShift
	KeyLAlt
	KeyLGUI
	KeyRCtrl
	KeyRShift
	KeyRAlt
	KeyRGUI
)

const (
	letterNames = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitNames  = "1234567890"
)

var fkeyNames = [...]string{
	"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12",
}

var namedKeys = map[Scancode]string{
	KeyEnter:        "Enter",
	KeyEscape:       "Escape",
	KeyBackspace:    "Backspace",
	KeyTab:          "Tab",
	KeySpace:        "Space",
	KeyMinus:        "-",
	KeyEquals:       "=",
	KeyLeftBracket:  "[",
	KeyRightBracket: "]",
	KeyBackslash:    "\\",
	KeyNonUSHash:    "#",
	KeySemicolon:    ";",
	KeyApostrophe:   "'",
	KeyGrave:        "`",
	KeyComma:        ",",
	KeyPeriod:       ".",
	KeySlash:        "/",
	KeyCapsLock:     "CapsLock",
	KeyPrintScreen:  "PrintScreen",
	KeyScrollLock:   "ScrollLock",
	KeyPause:        "Pause",
	KeyInsert:       "Insert",
	KeyHome:         "Home",
	KeyPageUp:       "PageUp",
	KeyDelete:       "Delete",
	KeyEnd:          "End",
	KeyPageDown:     "PageDown",
	KeyRight:        "Right",
	KeyLeft:         "Left",
	KeyDown:         "Down",
	KeyUp:           "Up",
	KeyNumLock:      "NumLock",
	KeyKPDivide:     "KP/",
	KeyKPMultiply:   "KP*",
	KeyKPMinus:      "KP-",
	KeyKPPlus:       "KP+",
	KeyKPEnter:      "KPEnter",
	KeyKPPeriod:     "KP.",
	KeyLCtrl:        "LCtrl",
	KeyLShift:       "LShift",
	KeyLAlt:         "LAlt",
	KeyLGUI:         "LGUI",
	KeyRCtrl:        "RCtrl",
	KeyRShift:       "RShift",
	KeyRAlt:         "RAlt",
	KeyRGUI:         "RGUI",
}

func PrimaryMod() Mods {
	if runtime.GOOS == "darwin" {
		return ModSuper
	}
	return ModCtrl
}

func WordMod() Mods {
	if runtime.GOOS == "darwin" {
		return ModAlt
	}
	return ModCtrl
}

func (m Mods) Primary() bool {
	return m&PrimaryMod() != 0
}

func (s Scancode) Valid() bool {
	return s != KeyUnknown && int(s) < ScancodeCount
}

func (s Scancode) String() string {
	switch {
	case s >= KeyA && s <= KeyZ:
		i := int(s - KeyA)
		return letterNames[i : i+1]
	case s >= Key1 && s <= Key0:
		i := int(s - Key1)
		return digitNames[i : i+1]
	case s >= KeyF1 && s <= KeyF12:
		return fkeyNames[s-KeyF1]
	case s >= KeyKP1 && s <= KeyKP0:
		i := int(s - KeyKP1)
		return "KP" + digitNames[i:i+1]
	}
	if n, ok := namedKeys[s]; ok {
		return n
	}
	return "Unknown"
}

type Keycode uint32

const keycodeScancodeBit Keycode = 1 << 30

func (k Keycode) IsScancodeBased() bool {
	return k&keycodeScancodeBit != 0
}

func (k Keycode) Rune() (rune, bool) {
	if k == 0 || k.IsScancodeBased() {
		return 0, false
	}
	return rune(k), true
}

type Mods uint16

const (
	ModShift Mods = 1 << iota
	ModCtrl
	ModAlt
	ModSuper
	ModCaps
	ModNum
)

var modNames = [...]struct {
	bit  Mods
	name string
}{
	{ModShift, "shift"},
	{ModCtrl, "ctrl"},
	{ModAlt, "alt"},
	{ModSuper, "super"},
	{ModCaps, "caps"},
	{ModNum, "num"},
}

func (m Mods) Has(o Mods) bool {
	return m&o == o
}

func (m Mods) String() string {
	if m == 0 {
		return "none"
	}
	var sb strings.Builder
	for _, mod := range modNames {
		if !m.Has(mod.bit) {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteByte('+')
		}
		sb.WriteString(mod.name)
	}
	return sb.String()
}

func ident(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return s != ""
}

func KeyName(k Scancode) (string, bool) {
	s := k.String()
	switch {
	case s == "Unknown":
		return "", false
	case len(s) == 1 && s[0] >= '0' && s[0] <= '9':
		return "Digit" + s, true
	case !ident(s):
		return "", false
	}
	return s, true
}
