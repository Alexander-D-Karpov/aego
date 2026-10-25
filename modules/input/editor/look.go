package editor

import (
	"aego/core/mathx"
	"aego/engine/i18n"
	"aego/modules/input"
	"aego/ui/core"
)

func keycap(ui *core.Context, id core.ID, r mathx.Rect, label string, armed bool) bool {
	th := &ui.Theme
	clicked := ui.Clicked(id, r)
	bg := th.Widget
	switch {
	case armed:
		bg = th.Accent
	case ui.Hot() == id:
		bg = th.WidgetHot
	}
	edge := mathx.Color{R: bg.R * 0.6, G: bg.G * 0.6, B: bg.B * 0.6, A: 1}
	ui.RoundRect(r, th.Radius.Button, edge)
	ui.RoundRect(mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H - 2}, th.Radius.Button, bg)
	col := th.Text
	if armed {
		col = th.Background
	}
	ui.TextIn(mathx.Rect{X: r.X, Y: r.Y - 1, W: r.W, H: r.H}, label, col, core.AlignCenter)
	return clicked
}

func capW(ui *core.Context, label string) float32 {
	return max(ui.TextSize(label).X+16, 28)
}

func chip(ui *core.Context, id core.ID, r mathx.Rect, label string, on bool) bool {
	th := &ui.Theme
	clicked := ui.Clicked(id, r)
	bg, col := th.Widget, th.TextDim
	if ui.Hot() == id {
		bg = th.WidgetHot
	}
	if on {
		bg, col = th.SelectionDim, th.Text
	}
	ui.RoundRect(r, r.H/2, bg)
	if on {
		ui.RoundRect(mathx.Rect{X: r.X + 6, Y: r.Y + r.H/2 - 3, W: 6, H: 6}, 3, th.Accent)
	}
	ui.Text(label, mathx.Vec2{X: r.X + 18, Y: ui.TextY(r)}, col)
	return clicked
}

func chips(ui *core.Context, id core.ID, names []string, on func(int) bool, toggle func(int)) bool {
	changed := false
	row := ui.Cursor().Next(0)
	x := row.X
	for i, n := range names {
		w := ui.TextSize(n).X + 28
		if x+w > row.X+row.W && x > row.X {
			row = ui.Cursor().Next(0)
			x = row.X
		}
		if chip(ui, core.IDSeq(id, i), mathx.Rect{X: x, Y: row.Y + 1, W: w, H: row.H - 2}, n, on(i)) {
			toggle(i)
			changed = true
		}
		x += w + 4
	}
	return changed
}

func badge(ui *core.Context, at mathx.Vec2, h float32, label string) float32 {
	th := &ui.Theme
	w := ui.TextSize(label).X + 12
	r := mathx.Rect{X: at.X, Y: at.Y + 3, W: w, H: h - 6}
	ui.RoundRect(r, r.H/2, th.Badge)
	ui.TextIn(r, label, th.TextDim, core.AlignCenter)
	return w
}

func deviceOf(b *input.Binding) string {
	switch b.Parts[0].Device {
	case input.DevKey:
		return i18n.Tr("i18n.input.keyboard", "Keyboard")
	case input.DevMouse, input.DevMouseAxis:
		return i18n.Tr("i18n.input.mouse", "Mouse")
	}
	return i18n.Tr("i18n.input.gamepad", "Gamepad")
}

type card struct {
	slot int
	top  float32
}

func beginCard(ui *core.Context) card {
	cur := ui.Cursor()
	c := card{slot: ui.List().ReserveBox(), top: cur.Y}
	cur.X += 8
	cur.W -= 16
	cur.Y += 6
	return c
}

func endCard(ui *core.Context, c card) {
	th := &ui.Theme
	cur := ui.Cursor()
	cur.X -= 8
	cur.W += 16
	cur.Y += 2
	r := mathx.Rect{X: cur.X, Y: c.top, W: cur.W, H: cur.Y - c.top}
	ui.List().PatchBox(c.slot, r, th.Radius.Card, th.CardBg, th.Border)
	cur.Y += th.Spacing + 4
}
