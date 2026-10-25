package widgets

import (
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/ui/core"
)

func colorBytes(col mathx.Color) [4]uint8 {
	enc := func(f float32) uint8 {
		return uint8(mathx.Clamp(mathx.LinearToSRGB(mathx.Clamp(f, 0, 1)), 0, 1)*255 + 0.5)
	}
	return [4]uint8{enc(col.R), enc(col.G), enc(col.B), uint8(mathx.Clamp(col.A, 0, 1)*255 + 0.5)}
}

func HexColor(col mathx.Color) string {
	b := colorBytes(col)
	v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	s := strconv.FormatUint(uint64(v), 16)
	return "#" + strings.Repeat("0", 8-len(s)) + s
}

func ParseHexColor(s string) (mathx.Color, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 6 {
		s += "ff"
	}
	if len(s) != 8 {
		return mathx.Color{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return mathx.Color{}, false
	}
	return mathx.ColorRGBA8(uint8(v>>24), uint8(v>>16), uint8(v>>8), uint8(v)), true
}

func Swatch(c *core.Context, r mathx.Rect, col mathx.Color, mixed bool) {
	th := &c.Theme
	if mixed {
		c.Rect(r, th.Widget)
		c.TextIn(r, th.Mixed, th.Text, core.AlignCenter)
		c.Outline(r, th.Border)
		return
	}
	opaque := col
	opaque.A = 1
	half := r.W / 2
	c.Rect(mathx.Rect{X: r.X, Y: r.Y, W: half, H: r.H}, opaque)
	right := mathx.Rect{X: r.X + half, Y: r.Y, W: r.W - half, H: r.H}
	cell := r.H / 2
	for y := 0; y < 2; y++ {
		for x := 0; float32(x)*cell < right.W; x++ {
			shade := mathx.Color{R: 0.8, G: 0.8, B: 0.8, A: 1}
			if (x+y)%2 == 1 {
				shade = mathx.Color{R: 0.45, G: 0.45, B: 0.45, A: 1}
			}
			w := min(cell, right.W-float32(x)*cell)
			c.Rect(mathx.Rect{X: right.X + float32(x)*cell, Y: right.Y + float32(y)*cell, W: w, H: cell}, shade)
		}
	}
	c.Rect(right, col)
	c.Outline(r, th.Border)
}

func SwatchHex(c *core.Context, r mathx.Rect, col mathx.Color, mixed bool) {
	th := &c.Theme
	c.Box(r, th.Radius.Field, th.Widget, th.Border)
	s := min(r.H-6, 16)
	sw := mathx.Rect{X: r.X + 4, Y: r.Y + (r.H-s)/2, W: s, H: s}
	text := th.Mixed
	if mixed {
		c.Rect(sw, th.Badge)
	} else {
		Swatch(c, sw, col, false)
		text = HexColor(col)
	}
	c.PushClip(r)
	c.Text(text, mathx.Vec2{X: sw.X + sw.W + 6, Y: c.TextY(r)}, th.Text)
	c.PopClip()
}

func ColorField(c *core.Context, id core.ID, r mathx.Rect, col *mathx.Color, mixed bool) Edit {
	var e Edit
	if c.Pressed(id, r) {
		if c.PopupOpen(id) {
			c.ClosePopup(id)
		} else {
			c.OpenPopup(id, mathx.Vec2{X: r.X, Y: r.Y + r.H})
		}
	}
	SwatchHex(c, r, *col, mixed)
	if !c.BeginPopup(id, 260) {
		return e
	}
	c.PushRawID(id)
	b := colorBytes(*col)
	vals := [4]float64{float64(b[0]), float64(b[1]), float64(b[2]), float64(b[3])}
	names := [4]string{"R", "G", "B", "A"}
	var channels Edit
	for i := range vals {
		fr := Field(c, names[i])
		channels = channels.Or(DragNumber(c, c.IDInt(i), fr, &vals[i], NumberOpts{
			Speed: 1, Min: 0, Max: 255, HasMin: true, HasMax: true, Int: true,
		}))
	}
	if channels.Changed {
		*col = mathx.ColorRGBA8(uint8(vals[0]), uint8(vals[1]), uint8(vals[2]), uint8(vals[3]))
	}
	e = e.Or(channels)
	hex := HexColor(*col)
	if _, committed := TextFieldRect(c, c.ID("hex"), Field(c, Tr("editor.widgets.hex", "Hex")), &hex, false); committed {
		if parsed, ok := ParseHexColor(hex); ok {
			*col = parsed
			e = e.Or(Edit{Changed: true, Begin: true, End: true})
		}
	}
	Swatch(c, c.Cursor().Next(0), *col, false)
	c.PopID()
	c.EndPopup()
	return e
}
