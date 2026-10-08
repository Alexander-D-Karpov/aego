package input

import "strings"

type Axis struct {
	Name        string
	Negative    Scancode
	Positive    Scancode
	AltNegative Scancode
	AltPositive Scancode
}

type AxisConfig struct {
	Axes []Axis
}

func DefaultAxes() AxisConfig {
	return AxisConfig{Axes: []Axis{
		{Name: "horizontal", Negative: KeyA, Positive: KeyD, AltNegative: KeyLeft, AltPositive: KeyRight},
		{Name: "vertical", Negative: KeyW, Positive: KeyS, AltNegative: KeyUp, AltPositive: KeyDown},
	}}
}

func (c AxisConfig) Lookup(name string) (Axis, bool) {
	for _, a := range c.Axes {
		if a.Name == name {
			return a, true
		}
	}
	return Axis{}, false
}

func (a Axis) Value(s *State) float32 {
	var v float32
	if s.KeyDown(a.Negative) || s.KeyDown(a.AltNegative) {
		v--
	}
	if s.KeyDown(a.Positive) || s.KeyDown(a.AltPositive) {
		v++
	}
	return v
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

func ParseKey(name string) (Scancode, bool) {
	name = strings.TrimSpace(name)
	for _, n := range []string{name, strings.TrimPrefix(name, "Key")} {
		for k := Scancode(1); k < ScancodeCount; k++ {
			if kn, ok := KeyName(k); ok && strings.EqualFold(kn, n) {
				return k, true
			}
		}
	}
	return KeyUnknown, false
}
