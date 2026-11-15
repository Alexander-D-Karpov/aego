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

// The marker stays inside the bar at both endpoints, leaving enough room to
// click exactly 0 and 255 even though Rect.Contains excludes the right edge.
func artAlphaAt(bar mathx.Rect, x float32) uint8 {
	travel := max(1, bar.W-4)
	t := mathx.Clamp((x-bar.X-2)/travel, 0, 1)
	return uint8(math.Round(float64(t * 255)))
}

func artAlphaRects(ui *core.Context) (mathx.Rect, mathx.Rect) {
	label := i18n.Tr("editor.art.color_alpha", "Alpha")
	if ui.Cursor().W < 230 {
		label = widgets.Elide(ui, label, ui.Cursor().W)
	}
	field := widgets.Field(ui, label)
	const numberWidth, gap, maxBarWidth = float32(54), float32(6), float32(240)
	if field.W < 120 {
		// Keep the track usable in a narrow dock instead of squeezing both
		// controls into an unreadable row.
		number := ui.Cursor().Next(0)
		number.W = min(number.W, numberWidth)
		return field, number
	}
	bar := field
	bar.W = min(maxBarWidth, field.W-numberWidth-gap)
	number := mathx.Rect{X: bar.X + bar.W + gap, Y: bar.Y, W: numberWidth, H: bar.H}
	return bar, number
}

func (p *artPanel) alphaPicker(ui *core.Context) {
	bar, number := artAlphaRects(ui)
	v := p.view
	id := ui.ID("color-alpha-bar")
	if ui.MouseOver(id, bar) {
		ui.SetCursorShape(core.CursorResizeEW)
	}
	alpha := float64(artColorBytes(v.color).A)
	numberID := ui.ID("color-alpha")
	edit := widgets.DragNumber(ui, numberID, number, &alpha, widgets.NumberOpts{Int: true, HasMin: true, Min: 0, HasMax: true, Max: 255, Speed: 1})
	if edit.Changed || edit.End {
		color := v.color
		color.A = float32(alpha) / 255
		p.setForegroundColor(color, edit.End)
		v.pickerLast = artColorBytes(v.color)
	}
	// The track supersedes uncommitted text in this same alpha field. Cancel
	// it explicitly: a focus-loss commit is otherwise delivered next frame,
	// after a press and release that can both occur in this frame.
	if _, active := ui.Drag(id, bar); active {
		if ui.MousePressed(input.MouseLeft) && ui.Focus() == numberID {
			ui.CancelFocus()
		}
		color := v.color
		color.A = float32(artAlphaAt(bar, ui.Mouse().X)) / 255
		p.setForegroundColor(color, ui.MouseReleased(input.MouseLeft))
		v.pickerLast = artColorBytes(v.color)
	}

	// Draw the same checker colors as a normal color swatch, then composite
	// the selected RGB over it from transparent to opaque. Rectangles are
	// bounded; the renderer does not currently have a gradient primitive.
	ui.PushClip(bar)
	const cell = float32(8)
	for y := float32(0); y < bar.H; y += cell {
		for x := float32(0); x < bar.W; x += cell {
			shade := float32(.8)
			if (int(x/cell)+int(y/cell))%2 != 0 {
				shade = .45
			}
			ui.Rect(mathx.Rect{X: bar.X + x, Y: bar.Y + y, W: min(cell, bar.W-x), H: min(cell, bar.H-y)}, mathx.Color{R: shade, G: shade, B: shade, A: 1})
		}
	}
	// Subpixel-width UI sprites can disappear on narrow tracks. Keep each
	// segment at least one logical pixel wide, while bounding draw commands.
	steps := max(2, min(64, int(bar.W)))
	for i := 0; i < steps; i++ {
		x0, x1 := bar.X+bar.W*float32(i)/float32(steps), bar.X+bar.W*float32(i+1)/float32(steps)
		color := v.color
		color.A = float32(i) / float32(steps-1)
		ui.Rect(mathx.Rect{X: x0, Y: bar.Y, W: x1 - x0, H: bar.H}, color)
	}
	ui.Outline(bar, ui.Theme.Border)
	t := float32(artColorBytes(v.color).A) / 255
	marker := mathx.Rect{X: bar.X + t*(bar.W-4), Y: bar.Y + 2, W: 4, H: max(0, bar.H-4)}
	ui.Rect(marker, mathx.Color{A: 1})
	ui.Rect(mathx.Rect{X: marker.X + 1, Y: marker.Y + 1, W: 2, H: max(0, marker.H-2)}, mathx.White)
	ui.PopClip()
	widgets.Tooltip(ui, id, i18n.Tr("editor.art.color_alpha", "Alpha"))
}

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
	p.alphaPicker(ui)
}
