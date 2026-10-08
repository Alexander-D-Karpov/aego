package widgets

import (
	"aego/core/mathx"
	"aego/render/texture"
	"aego/ui/core"
)

const stackWidth = 230

func LabelWidth(rowW float32) float32 {
	return mathx.Clamp(rowW*0.38, 60, 160)
}

func FieldRects(c *core.Context, label string) (mathx.Rect, mathx.Rect) {
	th := &c.Theme
	cur := c.Cursor()
	if cur.W < stackWidth {
		lr := cur.Next(c.LineHeight() + 2)
		c.PushClip(lr)
		c.Text(label, mathx.Vec2{X: lr.X, Y: lr.Y + 1}, th.TextDim)
		c.PopClip()
		return lr, cur.Next(0)
	}
	r := cur.Next(0)
	lw := LabelWidth(r.W)
	lr := mathx.Rect{X: r.X, Y: r.Y, W: lw - 4, H: r.H}
	c.PushClip(lr)
	c.Text(label, mathx.Vec2{X: lr.X, Y: c.TextY(lr)}, th.TextDim)
	c.PopClip()
	return lr, mathx.Rect{X: r.X + lw, Y: r.Y, W: r.W - lw, H: r.H}
}

func Field(c *core.Context, label string) mathx.Rect {
	_, v := FieldRects(c, label)
	return v
}

func ButtonRect(c *core.Context, id core.ID, r mathx.Rect, label string) bool {
	clicked := c.Clicked(id, r)
	c.Rect(r, c.WidgetColor(id))
	c.Outline(r, c.Theme.Border)
	c.TextIn(r, label, c.Theme.Text, core.AlignCenter)
	return clicked
}

func ToggleButton(c *core.Context, id core.ID, r mathx.Rect, label string, on *bool) bool {
	clicked := c.Clicked(id, r)
	if clicked {
		*on = !*on
	}
	bg := c.WidgetColor(id)
	if *on {
		bg = c.Theme.Selection
	}
	c.Rect(r, bg)
	c.Outline(r, c.Theme.Border)
	c.TextIn(r, label, c.Theme.Text, core.AlignCenter)
	return clicked
}

func ImageButton(c *core.Context, id core.ID, r mathx.Rect, reg texture.Region, tint mathx.Color) bool {
	clicked := c.Clicked(id, r)
	if c.Hot() == id || c.Active() == id {
		c.Rect(r, c.WidgetColor(id))
	}
	inner := mathx.Rect{X: r.X + 2, Y: r.Y + 2, W: r.W - 4, H: r.H - 4}
	if !reg.IsZero() {
		c.List().Texture(reg, inner, tint)
	}
	return clicked
}

func Selectable(c *core.Context, id core.ID, label string, selected bool) bool {
	r := c.Cursor().Next(0)
	c.FitPopup(c.TextSize(label).X + 12)
	clicked := c.Clicked(id, r)
	if selected {
		c.List().Rect(r, c.Theme.WidgetDown)
	} else if c.Hot() == id {
		c.List().Rect(r, c.Theme.WidgetHot)
	}
	c.List().Text(label, mathx.Vec2{X: r.X + 4, Y: textY(c, r)}, c.Theme.Text)
	return clicked
}

func SearchField(c *core.Context, id core.ID, r mathx.Rect, q *string) bool {
	th := &c.Theme
	changed := false
	field := r
	if *q != "" {
		cr := mathx.Rect{X: r.X + r.W - r.H, Y: r.Y, W: r.H, H: r.H}
		cid := core.IDSeq(id, 7)
		if c.Clicked(cid, cr) {
			*q = ""
			c.CancelFocus()
			changed = true
		}
		field.W -= r.H
		defer c.TextIn(cr, glyph(c, '×', "x"), th.TextDim, core.AlignCenter)
	}
	if ch, _ := TextFieldRect(c, id, field, q, true); ch {
		changed = true
	}
	if *q == "" && c.Focus() != id {
		c.Text("Search", mathx.Vec2{X: r.X + 6, Y: c.TextY(r)}, th.TextDisabled)
	}
	return changed
}

func Splitter(c *core.Context, id core.ID, r mathx.Rect, vertical bool, ratio *float32) (mathx.Rect, mathx.Rect) {
	th := &c.Theme
	g := th.SplitterW
	*ratio = mathx.Clamp(*ratio, 0.05, 0.95)
	var a, b, h mathx.Rect
	if !vertical {
		w := (r.W - g) * *ratio
		a = mathx.Rect{X: r.X, Y: r.Y, W: w, H: r.H}
		h = mathx.Rect{X: r.X + w, Y: r.Y, W: g, H: r.H}
		b = mathx.Rect{X: r.X + w + g, Y: r.Y, W: r.W - w - g, H: r.H}
	} else {
		hh := (r.H - g) * *ratio
		a = mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: hh}
		h = mathx.Rect{X: r.X, Y: r.Y + hh, W: r.W, H: g}
		b = mathx.Rect{X: r.X, Y: r.Y + hh + g, W: r.W, H: r.H - hh - g}
	}
	if d, ok := c.Drag(id, h); ok {
		if !vertical && r.W > g {
			*ratio = mathx.Clamp(*ratio+d.X/(r.W-g), 0.05, 0.95)
		}
		if vertical && r.H > g {
			*ratio = mathx.Clamp(*ratio+d.Y/(r.H-g), 0.05, 0.95)
		}
	}
	col := th.Background
	if c.Hot() == id || c.Active() == id {
		col = th.AccentDim
		if vertical {
			c.SetCursorShape(core.CursorResizeNS)
		} else {
			c.SetCursorShape(core.CursorResizeEW)
		}
	}
	c.Rect(h, col)
	return a, b
}
