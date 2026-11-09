package editor

import (
	"math"

	"aego/art"
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/widgets"
)

// HSV is calculated in the same sRGB byte space exposed by the Art document,
// not in the linear-light UI representation. Keep hue and saturation at black
// or gray so dragging back from an achromatic color stays predictable.
func artColorHSV(c art.Color, hue, saturation float32) (float32, float32, float32) {
	r, g, b := float32(c.R)/255, float32(c.G)/255, float32(c.B)/255
	value := max(r, max(g, b))
	delta := value - min(r, min(g, b))
	if value > 0 {
		saturation = delta / value
	}
	if delta > 0 {
		switch value {
		case r:
			hue = (g - b) / delta
		case g:
			hue = (b-r)/delta + 2
		default:
			hue = (r-g)/delta + 4
		}
		hue /= 6
		if hue < 0 {
			hue++
		}
	}
	return hue, saturation, value
}

func artHSVColor(hue, saturation, value float32, alpha uint8) art.Color {
	hue -= float32(math.Floor(float64(hue)))
	saturation, value = mathx.Clamp(saturation, 0, 1), mathx.Clamp(value, 0, 1)
	sector := hue * 6
	i := int(sector)
	f := sector - float32(i)
	p, q, t := value*(1-saturation), value*(1-saturation*f), value*(1-saturation*(1-f))
	var r, g, b float32
	switch i {
	case 0:
		r, g, b = value, t, p
	case 1:
		r, g, b = q, value, p
	case 2:
		r, g, b = p, value, t
	case 3:
		r, g, b = p, q, value
	case 4:
		r, g, b = t, p, value
	default:
		r, g, b = value, p, q
	}
	byteOf := func(v float32) uint8 { return uint8(mathx.Clamp(v, 0, 1)*255 + .5) }
	return art.Color{R: byteOf(r), G: byteOf(g), B: byteOf(b), A: alpha}
}

func artPickerColor(c art.Color) mathx.Color { return mathx.ColorRGBA8(c.R, c.G, c.B, c.A) }

// The picker changes the active foreground only. Palette changes continue to
// use the existing explicit, undoable actions in the shared Art session.
func (p *artPanel) colorPicker(ui *core.Context) {
	cur, v := ui.Cursor(), p.view
	if cur.W < 32 {
		return
	}
	current := artColorBytes(v.color)
	if !v.pickerReady {
		v.pickerSaturation = 1
	}
	if !v.pickerReady || current != v.pickerLast {
		v.pickerHue, v.pickerSaturation, _ = artColorHSV(current, v.pickerHue, v.pickerSaturation)
		v.pickerLast, v.pickerReady = current, true
	}
	hue, saturation, value := artColorHSV(current, v.pickerHue, v.pickerSaturation)
	side := min(float32(240), cur.W-24)
	row := cur.Next(side)
	sv := mathx.Rect{X: row.X, Y: row.Y, W: side, H: side}
	bar := mathx.Rect{X: row.X + side + 4, Y: row.Y, W: 20, H: side}
	if _, active := ui.Drag(ui.ID("color-sv"), sv); active {
		saturation = mathx.Clamp((ui.Mouse().X-sv.X)/sv.W, 0, 1)
		value = 1 - mathx.Clamp((ui.Mouse().Y-sv.Y)/sv.H, 0, 1)
		v.pickerHue, v.pickerSaturation = hue, saturation
		p.setForegroundColor(artPickerColor(artHSVColor(hue, saturation, value, current.A)), ui.MouseReleased(input.MouseLeft))
		v.pickerLast = artColorBytes(v.color)
	}
	if _, active := ui.Drag(ui.ID("color-hue"), bar); active {
		hue = mathx.Clamp((ui.Mouse().Y-bar.Y)/bar.H, 0, 1)
		v.pickerHue, v.pickerSaturation = hue, saturation
		p.setForegroundColor(artPickerColor(artHSVColor(hue, saturation, value, current.A)), ui.MouseReleased(input.MouseLeft))
		v.pickerLast = artColorBytes(v.color)
	}
	// The existing UI renderer has no gradient primitive. A bounded grid keeps
	// the picker self-contained and does not allocate textures every frame.
	const steps = 20
	for y := 0; y < steps; y++ {
		for x := 0; x < steps; x++ {
			x0, x1 := sv.X+sv.W*float32(x)/steps, sv.X+sv.W*float32(x+1)/steps
			y0, y1 := sv.Y+sv.H*float32(y)/steps, sv.Y+sv.H*float32(y+1)/steps
			col := artHSVColor(hue, float32(x)/(steps-1), 1-float32(y)/(steps-1), 255)
			ui.Rect(mathx.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, artPickerColor(col))
		}
		col := artHSVColor(float32(y)/(steps-1), 1, 1, 255)
		y0, y1 := bar.Y+bar.H*float32(y)/steps, bar.Y+bar.H*float32(y+1)/steps
		ui.Rect(mathx.Rect{X: bar.X, Y: y0, W: bar.W, H: y1 - y0}, artPickerColor(col))
	}
	ui.Outline(sv, ui.Theme.Border)
	ui.Outline(bar, ui.Theme.Border)
	marker := mathx.Rect{X: sv.X + mathx.Clamp(saturation*sv.W-4, 0, max(0, sv.W-8)), Y: sv.Y + mathx.Clamp((1-value)*sv.H-4, 0, max(0, sv.H-8)), W: min(8, sv.W), H: min(8, sv.H)}
	ui.Outline(marker, mathx.Color{A: 1})
	marker = mathx.Rect{X: marker.X + 1, Y: marker.Y + 1, W: max(0, marker.W-2), H: max(0, marker.H-2)}
	ui.Outline(marker, mathx.White)
	hy := bar.Y + mathx.Clamp(hue*bar.H-2, 0, max(0, bar.H-4))
	ui.Outline(mathx.Rect{X: bar.X, Y: hy, W: bar.W, H: min(4, bar.H)}, mathx.White)
	widgets.Tooltip(ui, ui.ID("color-sv"), i18n.Tr("editor.art.color_sv_hint", "Drag to choose saturation and brightness"))
	widgets.Tooltip(ui, ui.ID("color-hue"), i18n.Tr("editor.art.color_hue_hint", "Drag to choose hue"))
	alpha := float64(artColorBytes(v.color).A)
	edit := widgets.DragNumber(ui, ui.ID("color-alpha"), widgets.Field(ui, i18n.Tr("editor.art.color_alpha", "Alpha")), &alpha, widgets.NumberOpts{Int: true, HasMin: true, Min: 0, HasMax: true, Max: 255, Speed: 1})
	if edit.Changed || edit.End {
		color := v.color
		color.A = float32(alpha) / 255
		p.setForegroundColor(color, edit.End)
		v.pickerLast = artColorBytes(v.color)
	}
}
