package widgets

import (
	"fmt"
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/ui/core"
)

func textY(c *core.Context, r mathx.Rect) float32 {
	h := c.TextSize("M").Y
	return r.Y + (r.H-h)/2
}

func Label(c *core.Context, s string) {
	r := c.Cursor().Next(0)
	c.List().Text(s, mathx.Vec2{X: r.X, Y: textY(c, r)}, c.Theme.Text)
}

func Labelf(c *core.Context, format string, a ...any) {
	Label(c, fmt.Sprintf(format, a...))
}

func LabelDim(c *core.Context, s string) {
	r := c.Cursor().Next(0)
	c.List().Text(s, mathx.Vec2{X: r.X, Y: textY(c, r)}, c.Theme.TextDim)
}

func LabelWrapped(c *core.Context, s string, dim bool) {
	width := c.Cursor().W
	color := c.Theme.Text
	if dim {
		color = c.Theme.TextDim
	}
	for len(s) > 0 {
		line := fitLine(c, s, width)
		r := c.Cursor().Next(0)
		c.List().Text(line, mathx.Vec2{X: r.X, Y: textY(c, r)}, color)
		s = strings.TrimLeft(s[len(line):], " ")
	}
}

func fitLine(c *core.Context, s string, width float32) string {
	if c.TextSize(s).X <= width {
		return s
	}
	cut := 0
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			continue
		}
		if c.TextSize(s[:i]).X > width {
			break
		}
		cut = i
	}
	if cut > 0 {
		return s[:cut]
	}
	for i := 1; i <= len(s); i++ {
		if c.TextSize(s[:i]).X > width {
			return s[:max(1, i-1)]
		}
	}
	return s
}

func Separator(c *core.Context) {
	cur := c.Cursor()
	r := cur.Next(c.Theme.Spacing)
	c.List().Rect(mathx.Rect{X: r.X, Y: r.Y + r.H/2, W: r.W, H: 1}, c.Theme.Border)
}

func Button(c *core.Context, label string) bool {
	id := c.ID(label)
	r := c.Cursor().Next(0)
	clicked := c.Clicked(id, r)
	c.List().Rect(r, c.WidgetColor(id))
	c.List().RectOutline(r, c.Theme.Border, 1)
	size := c.TextSize(label)
	c.List().Text(label, mathx.Vec2{X: r.X + (r.W-size.X)/2, Y: textY(c, r)}, c.Theme.Text)
	return clicked
}

func Checkbox(c *core.Context, label string, v *bool) bool {
	id := c.ID(label)
	r := c.Cursor().Next(0)
	box := mathx.Rect{X: r.X, Y: r.Y + 2, W: r.H - 4, H: r.H - 4}
	changed := false
	if c.Clicked(id, r) {
		*v = !*v
		changed = true
	}
	c.List().Rect(box, c.WidgetColor(id))
	c.List().RectOutline(box, c.Theme.Border, 1)
	if *v {
		inner := mathx.Rect{X: box.X + 3, Y: box.Y + 3, W: box.W - 6, H: box.H - 6}
		c.List().Rect(inner, c.Theme.Accent)
	}
	c.List().Text(label, mathx.Vec2{X: r.X + r.H + 4, Y: textY(c, r)}, c.Theme.Text)
	return changed
}

func Slider(c *core.Context, label string, v *float32, lo, hi float32) bool {
	id := c.ID(label)
	r := c.Cursor().Next(0)
	if hi <= lo {
		hi = lo + 1
	}
	changed := false
	if delta, dragging := c.Drag(id, r); dragging {
		if delta.X != 0 {
			*v = mathx.Clamp(*v+delta.X/r.W*(hi-lo), lo, hi)
			changed = true
		}
	}
	t := mathx.Clamp((*v-lo)/(hi-lo), 0, 1)
	c.List().Rect(r, c.Theme.Widget)
	c.List().Rect(mathx.Rect{X: r.X, Y: r.Y, W: r.W * t, H: r.H}, c.WidgetColor(id))
	c.List().RectOutline(r, c.Theme.Border, 1)
	buf := make([]byte, 0, 32)
	buf = append(buf, label...)
	buf = append(buf, ' ')
	buf = strconv.AppendFloat(buf, float64(*v), 'f', 2, 32)
	c.List().Text(string(buf), mathx.Vec2{X: r.X + 4, Y: textY(c, r)}, c.Theme.Text)
	return changed
}
