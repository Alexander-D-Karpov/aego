package input

import (
	"strings"

	raw "aego/platform/input"
)

type Device uint8

const (
	DevNone Device = iota
	DevKey
	DevMouse
	DevMouseAxis
	DevPadButton
	DevPadAxis
	DevStick
)

type Control struct {
	Device Device
	Code   uint16
}

const (
	MouseX uint16 = iota
	MouseY
	MouseWheel
)

const (
	StickLeft uint16 = iota
	StickRight
)

var (
	mouseButtons = [...]struct {
		name string
		b    raw.Button
	}{{"Left", raw.MouseLeft}, {"Right", raw.MouseRight}, {"Middle", raw.MouseMiddle}, {"X1", raw.MouseX1}, {"X2", raw.MouseX2}}
	mouseAxes = [...]string{"X", "Y", "Wheel"}
	sticks    = [...]string{"LeftStick", "RightStick"}
	keyByName = keyNames()
)

func keyNames() map[string]raw.Scancode {
	m := map[string]raw.Scancode{}
	for k := raw.Scancode(1); k < raw.ScancodeCount; k++ {
		if n, ok := raw.KeyName(k); ok {
			m[n] = k
		}
	}
	return m
}

func (c Control) IsZero() bool {
	return c.Device == DevNone
}

func (c Control) Pad() bool {
	return c.Device >= DevPadButton
}

func (c Control) Analog() bool {
	return c.Device == DevPadAxis || c.Device == DevStick
}

func ParseControl(s string) (Control, bool) {
	dev, name, ok := strings.Cut(s, ".")
	if !ok {
		return Control{}, false
	}
	switch dev {
	case "Key":
		if k, ok := keyByName[name]; ok {
			return Control{DevKey, uint16(k)}, true
		}
	case "Mouse":
		for _, m := range mouseButtons {
			if m.name == name {
				return Control{DevMouse, uint16(m.b)}, true
			}
		}
		for i, n := range mouseAxes {
			if n == name {
				return Control{DevMouseAxis, uint16(i)}, true
			}
		}
	case "Pad":
		for i, n := range sticks {
			if n == name {
				return Control{DevStick, uint16(i)}, true
			}
		}
		for b := raw.PadButton(0); b < raw.PadButtonCount; b++ {
			if b.String() == name {
				return Control{DevPadButton, uint16(b)}, true
			}
		}
		for a := raw.PadAxis(0); a < raw.PadAxisCount; a++ {
			if a.String() == name {
				return Control{DevPadAxis, uint16(a)}, true
			}
		}
	}
	return Control{}, false
}

func (c Control) name() string {
	switch c.Device {
	case DevKey:
		n, _ := raw.KeyName(raw.Scancode(c.Code))
		return n
	case DevMouse:
		for _, m := range mouseButtons {
			if uint16(m.b) == c.Code {
				return m.name
			}
		}
	case DevMouseAxis:
		if int(c.Code) < len(mouseAxes) {
			return mouseAxes[c.Code]
		}
	case DevPadButton:
		return raw.PadButton(c.Code).String()
	case DevPadAxis:
		return raw.PadAxis(c.Code).String()
	case DevStick:
		if int(c.Code) < len(sticks) {
			return sticks[c.Code]
		}
	}
	return ""
}

func (c Control) String() string {
	switch c.Device {
	case DevKey:
		return "Key." + c.name()
	case DevMouse, DevMouseAxis:
		return "Mouse." + c.name()
	case DevPadButton, DevPadAxis, DevStick:
		return "Pad." + c.name()
	}
	return ""
}

func (c Control) Label() string {
	if c.Device == DevMouse || c.Device == DevMouseAxis {
		return "Mouse " + c.name()
	}
	return c.name()
}
