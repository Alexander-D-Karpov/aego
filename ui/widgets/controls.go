package widgets

import (
	"strings"

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
		flash(c, label, lr)
		c.PushClip(lr)
		c.Text(label, mathx.Vec2{X: lr.X, Y: lr.Y + 1}, th.TextDim)
		c.PopClip()
		return lr, cur.Next(0)
	}
	r := cur.Next(0)
	flash(c, label, r)
	lw := LabelWidth(r.W)
	lr := mathx.Rect{X: r.X, Y: r.Y, W: lw - 4, H: r.H}
	c.PushClip(lr)
	c.Text(label, mathx.Vec2{X: lr.X, Y: c.TextY(lr)}, th.TextDim)
	c.PopClip()
	return lr, mathx.Rect{X: r.X + lw, Y: r.Y, W: r.W - lw, H: r.H}
}

func flash(c *core.Context, label string, r mathx.Rect) {
	if c.Flash == "" || label == "" {
		return
	}
	a, b := strings.ToLower(c.Flash), strings.ToLower(label)
	if !strings.HasPrefix(a, b) && !strings.HasPrefix(b, a) {
		return
	}
	k := 1 - float32(c.Time-c.FlashAt)/2
	if k <= 0 {
		return
	}
	col := c.Theme.Accent
	col.A = 0.28 * k
	c.RoundRect(mathx.Rect{X: r.X - 4, Y: r.Y - 1, W: r.W + 8, H: r.H + 2}, c.Theme.Radius.Field, col)
	if !c.FlashSeen {
		c.FlashY, c.FlashSeen = r.Y, true
	}
}

func Field(c *core.Context, label string) mathx.Rect {
	_, v := FieldRects(c, label)
	return v
}

func ButtonRect(c *core.Context, id core.ID, r mathx.Rect, label string) bool {
	clicked := c.Clicked(id, r)
	c.Box(r, c.Theme.Radius.Button, c.WidgetColor(id), c.Theme.Border)
	c.TextIn(r, label, c.Theme.Text, core.AlignCenter)
	return clicked
}

func IconButtonRect(c *core.Context, id core.ID, r mathx.Rect, icon string) bool {
	clicked := c.Clicked(id, r)
	c.Box(r, c.Theme.Radius.Button, c.WidgetColor(id), c.Theme.Border)
	IconIn(c, r, icon, c.Theme.Text)
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
	c.Box(r, c.Theme.Radius.Button, bg, c.Theme.Border)
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
		c.RoundRect(r, c.Theme.Radius.Menu, c.Theme.WidgetDown)
	} else if c.Hot() == id {
		c.RoundRect(r, c.Theme.Radius.Menu, c.Theme.WidgetHot)
	}
	c.List().Text(label, mathx.Vec2{X: r.X + 4, Y: textY(c, r)}, c.Theme.Text)
	return clicked
}

func SelectableIcon(c *core.Context, id core.ID, icon, label string, selected bool) bool {
	th := &c.Theme
	r := c.Cursor().Next(0)
	c.FitPopup(c.TextSize(label).X + 34)
	clicked := c.Clicked(id, r)
	if selected {
		c.RoundRect(r, th.Radius.Menu, th.WidgetDown)
	} else if c.Hot() == id {
		c.RoundRect(r, th.Radius.Menu, th.WidgetHot)
	}
	if HasIcon(c, icon) {
		IconAt(c, mathx.Rect{X: r.X + 6, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, icon, th.TextDim)
	}
	c.List().Text(label, mathx.Vec2{X: r.X + 28, Y: textY(c, r)}, th.Text)
	return clicked
}

func HeaderIcon(c *core.Context, icon, label string, dim bool) {
	th := &c.Theme
	r := c.Cursor().Next(0)
	col := th.TextDim
	if dim {
		col = th.TextDisabled
	}
	x := r.X + 4
	if HasIcon(c, icon) {
		IconAt(c, mathx.Rect{X: x, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, icon, col)
		x += 22
	}
	c.List().Text(label, mathx.Vec2{X: x, Y: textY(c, r)}, col)
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
		defer IconIn(c, cr, "ui.close", th.TextDim)
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

func Switch(c *core.Context, id core.ID, r mathx.Rect, on *bool, label string) bool {
	th := &c.Theme
	clicked := c.Clicked(id, r)
	if clicked {
		*on = !*on
	}
	h := min(r.H-6, 16)
	pill := mathx.Rect{X: r.X, Y: float32(int(r.Y + (r.H-h)/2)), W: h * 1.8, H: h}
	bg := th.Widget
	if c.Hot() == id {
		bg = th.WidgetHot
	}
	if *on {
		bg = th.Accent
	}
	c.RoundRect(pill, h/2, bg)
	k := h - 4
	kx := pill.X + 2
	if *on {
		kx = pill.X + pill.W - k - 2
	}
	c.RoundRect(mathx.Rect{X: kx, Y: pill.Y + 2, W: k, H: k}, k/2, th.Text)
	if label != "" {
		col := th.TextDim
		if *on {
			col = th.Text
		}
		c.Text(label, mathx.Vec2{X: pill.X + pill.W + 8, Y: c.TextY(r)}, col)
	}
	return clicked
}
