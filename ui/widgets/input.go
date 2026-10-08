package widgets

import (
	"strconv"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

func TextInput(c *core.Context, id core.ID, buf *string, label string) bool {
	r := c.Cursor().Next(0)
	focused := c.Focus() == id
	if c.Clicked(id, r) {
		c.SetFocus(id)
		focused = true
	}
	changed := false
	if focused {
		if text := c.TextInput(); len(text) > 0 {
			*buf += string(text)
			changed = true
		}
		if c.KeyRepeat(input.KeyBackspace) && len(*buf) > 0 {
			*buf = (*buf)[:len(*buf)-1]
			changed = true
		}
		if c.KeyRepeat(input.KeyEscape) || c.KeyRepeat(input.KeyEnter) {
			c.SetFocus(0)
		}
	}
	bg := c.Theme.Widget
	if focused {
		bg = c.Theme.WidgetDown
	}
	c.List().Rect(r, bg)
	border := c.Theme.Border
	if focused {
		border = c.Theme.Accent
	}
	c.List().RectOutline(r, border, 1)
	text := *buf
	if focused {
		text += "_"
	}
	c.List().Text(text, mathx.Vec2{X: r.X + 4, Y: textY(c, r)}, c.Theme.Text)
	if label != "" {
		c.List().Text(label, mathx.Vec2{X: r.X + r.W + 6, Y: textY(c, r)}, c.Theme.TextDim)
	}
	return changed
}

func TreeNode(c *core.Context, id core.ID, label string, open *bool, selected bool) bool {
	r := c.Cursor().Next(0)
	clicked := c.Clicked(id, r)
	arrowRect := mathx.Rect{X: r.X, Y: r.Y, W: r.H, H: r.H}
	if clicked && c.Mouse().X < arrowRect.X+arrowRect.W {
		*open = !*open
		clicked = false
	}
	if selected {
		c.List().Rect(r, c.Theme.WidgetDown)
	} else if c.Hot() == id {
		c.List().Rect(r, c.Theme.WidgetHot)
	}
	arrow := "+"
	if *open {
		arrow = "-"
	}
	c.List().Text(arrow, mathx.Vec2{X: r.X + 3, Y: textY(c, r)}, c.Theme.TextDim)
	c.List().Text(label, mathx.Vec2{X: r.X + r.H, Y: textY(c, r)}, c.Theme.Text)
	return clicked
}

func DragFloat(c *core.Context, id core.ID, label string, v *float32, speed float32) bool {
	r := c.Cursor().Next(0)
	changed := false
	if delta, dragging := c.Drag(id, r); dragging && delta.X != 0 {
		*v += delta.X * speed
		changed = true
	}
	c.List().Rect(r, c.WidgetColor(id))
	c.List().RectOutline(r, c.Theme.Border, 1)
	buf := make([]byte, 0, 48)
	buf = append(buf, label...)
	buf = append(buf, ' ')
	buf = strconv.AppendFloat(buf, float64(*v), 'g', 4, 32)
	c.List().Text(string(buf), mathx.Vec2{X: r.X + 4, Y: textY(c, r)}, c.Theme.Text)
	return changed
}

func DragInt(c *core.Context, id core.ID, label string, v *int32) bool {
	f := float32(*v)
	if DragFloat(c, id, label, &f, 0.25) {
		*v = int32(f)
		return true
	}
	return false
}
