package input

import (
	"strconv"
	"strings"
)

type Shape uint8

const (
	Single Shape = iota
	Pair
	Quad
)

type Binding struct {
	Shape    Shape
	Parts    [4]Control
	Mod      Control
	Scale    float32
	Invert   bool
	Deadzone float32
	text     string
	glyph    string
}

func (b *Binding) count() int {
	switch b.Shape {
	case Pair:
		return 2
	case Quad:
		return 4
	}
	return 1
}

func ParseBinding(s string) (Binding, error) {
	var b Binding
	f := strings.Fields(s)
	if len(f) == 0 {
		return b, bad("empty binding")
	}
	expr := f[0]
	if m, rest, ok := strings.Cut(expr, "+"); ok {
		c, ok := ParseControl(m)
		if !ok {
			return b, bad("unknown control " + m)
		}
		b.Mod, expr = c, rest
	}
	parts := strings.Split(expr, "/")
	switch len(parts) {
	case 1:
		b.Shape = Single
	case 2:
		b.Shape = Pair
	case 4:
		b.Shape = Quad
	default:
		return b, bad("binding " + s + " needs one, two or four controls")
	}
	for i, p := range parts {
		c, ok := ParseControl(p)
		if !ok {
			return b, bad("unknown control " + p)
		}
		if c.Device == DevStick && b.Shape != Single {
			return b, bad("a stick cannot be part of a composite: " + s)
		}
		b.Parts[i] = c
	}
	for _, o := range f[1:] {
		k, v, _ := strings.Cut(o, "=")
		switch k {
		case "invert":
			b.Invert = true
		case "scale", "deadzone":
			x, err := strconv.ParseFloat(v, 32)
			if err != nil {
				return b, bad("binding " + s + ": " + k + " needs a number")
			}
			if k == "scale" {
				b.Scale = float32(x)
			} else {
				b.Deadzone = float32(x)
			}
		default:
			return b, bad("binding " + s + ": unknown option " + o)
		}
	}
	b.finish()
	return b, nil
}

func (b *Binding) finish() {
	var t, g strings.Builder
	if !b.Mod.IsZero() {
		t.WriteString(b.Mod.String() + "+")
		g.WriteString(b.Mod.Label() + "+")
	}
	for i := 0; i < b.count(); i++ {
		if i > 0 {
			t.WriteByte('/')
			g.WriteByte('/')
		}
		t.WriteString(b.Parts[i].String())
		g.WriteString(b.Parts[i].Label())
	}
	if b.Invert {
		t.WriteString(" invert")
	}
	if b.Scale != 0 && b.Scale != 1 {
		t.WriteString(" scale=" + strconv.FormatFloat(float64(b.Scale), 'g', -1, 32))
	}
	if b.Deadzone > 0 {
		t.WriteString(" deadzone=" + strconv.FormatFloat(float64(b.Deadzone), 'g', -1, 32))
	}
	b.text = t.String()
	b.glyph = "[" + g.String() + "]"
}

func (b *Binding) String() string {
	return b.text
}

func (b *Binding) Glyph() string {
	return b.glyph
}

func (b *Binding) Pad() bool {
	for i := 0; i < b.count(); i++ {
		if b.Parts[i].Pad() {
			return true
		}
	}
	return false
}

func (b *Binding) analog() bool {
	for i := 0; i < b.count(); i++ {
		if b.Parts[i].Analog() {
			return true
		}
	}
	return false
}

func (b *Binding) Uses(c Control) bool {
	if b.Mod == c {
		return true
	}
	for i := 0; i < b.count(); i++ {
		if b.Parts[i] == c {
			return true
		}
	}
	return false
}

func (b *Binding) Refresh() {
	b.finish()
}

func (b *Binding) Fits(k Kind) bool {
	return b.fits(k)
}

func PartCount(s Shape) int {
	switch s {
	case Pair:
		return 2
	case Quad:
		return 4
	}
	return 1
}

func (b *Binding) fits(k Kind) bool {
	stick := b.Shape == Single && b.Parts[0].Device == DevStick
	switch k {
	case Button:
		return b.Shape == Single && !stick
	case Axis1D:
		return (b.Shape == Single && !stick) || b.Shape == Pair
	case Axis2D:
		return stick || b.Shape == Quad
	}
	return false
}
