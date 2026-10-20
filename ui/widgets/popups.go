package widgets

import (
	"math/bits"
	"strconv"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/draw"
)

func glyph(c *core.Context, r rune, fallback string) string {
	if f := c.Font(); f != nil && f.Has(r) {
		return string(r)
	}
	return fallback
}

func Glyph(c *core.Context, r rune, fallback string) string {
	return glyph(c, r, fallback)
}

func dropdownBox(c *core.Context, id core.ID, r mathx.Rect, label string) {
	th := &c.Theme
	c.Box(r, th.Radius.Field, c.WidgetColor(id), th.Border)
	inner := mathx.Rect{X: r.X, Y: r.Y, W: r.W - r.H, H: r.H}
	c.PushClip(inner)
	c.TextIn(inner, label, th.Text, core.AlignLeft)
	c.PopClip()
	Chevron(c, mathx.Rect{X: r.X + r.W - r.H, Y: r.Y, W: r.H, H: r.H}, true, th.TextDim)
}

func togglePopup(c *core.Context, id core.ID, r mathx.Rect) {
	if c.Pressed(id, r) {
		if c.PopupOpen(id) {
			c.ClosePopup(id)
		} else {
			c.OpenPopup(id, mathx.Vec2{X: r.X, Y: r.Y + r.H})
		}
	}
}

func Combo(c *core.Context, id core.ID, r mathx.Rect, cur *int, opts []string, mixed bool) bool {
	togglePopup(c, id, r)
	label := c.Theme.Mixed
	if !mixed && *cur >= 0 && *cur < len(opts) {
		label = opts[*cur]
	}
	dropdownBox(c, id, r, label)
	changed := false
	if c.BeginPopup(id, max(r.W, 140)) {
		c.PushRawID(id)
		for i, o := range opts {
			if Selectable(c, c.IDInt(i), o, !mixed && i == *cur) {
				*cur = i
				changed = true
				c.ClosePopup(id)
			}
		}
		c.PopID()
		c.EndPopup()
	}
	return changed
}

func allBits(n int) uint64 {
	if n >= 64 {
		return ^uint64(0)
	}
	return uint64(1)<<uint(n) - 1
}

func maskSummary(mask uint64, names []string, n int, mixed bool, th *core.Theme) string {
	switch {
	case mixed:
		return th.Mixed
	case mask == 0:
		return "Nothing"
	case mask == ^uint64(0) || mask == allBits(n):
		return "Everything"
	case bits.OnesCount64(mask) == 1:
		i := bits.TrailingZeros64(mask)
		if i < len(names) {
			return names[i]
		}
		return "Layer " + strconv.Itoa(i)
	}
	if len(names) > 0 {
		return strconv.Itoa(bits.OnesCount64(mask)) + " selected"
	}
	return strconv.Itoa(bits.OnesCount64(mask)) + " layers"
}

func CheckRow(c *core.Context, id core.ID, label string, on bool) bool {
	th := &c.Theme
	r := c.Cursor().Next(0)
	clicked := c.Clicked(id, r)
	if c.Hot() == id {
		c.RoundRect(r, th.Radius.Menu, th.WidgetHot)
	}
	box := mathx.Rect{X: r.X + 2, Y: r.Y + 4, W: r.H - 8, H: r.H - 8}
	if on {
		c.RoundRect(box, th.Radius.Field-1, th.Accent)
		CheckMark(c, box, th.Background)
	} else {
		c.Box(box, th.Radius.Field-1, th.Widget, th.Border)
	}
	c.Text(label, mathx.Vec2{X: box.X + box.W + 6, Y: c.TextY(r)}, th.Text)
	return clicked
}

func MaskField(c *core.Context, id core.ID, r mathx.Rect, mask *uint64, names []string, mixed bool) bool {
	n := len(names)
	if n == 0 {
		n = 32
	}
	togglePopup(c, id, r)
	dropdownBox(c, id, r, maskSummary(*mask, names, n, mixed, &c.Theme))
	changed := false
	if c.BeginPopup(id, max(r.W, 180)) {
		c.PushRawID(id)
		if Selectable(c, c.ID("everything"), "Everything", !mixed && *mask == allBits(n)) {
			*mask = allBits(n)
			changed = true
		}
		if Selectable(c, c.ID("nothing"), "Nothing", !mixed && *mask == 0) {
			*mask = 0
			changed = true
		}
		Separator(c)
		for i := 0; i < n; i++ {
			name := "Layer " + strconv.Itoa(i)
			if i < len(names) && names[i] != "" {
				name = names[i]
			}
			bit := uint64(1) << uint(i)
			if CheckRow(c, c.IDInt(i), name, *mask&bit != 0) {
				*mask ^= bit
				changed = true
			}
		}
		c.PopID()
		c.EndPopup()
	}
	return changed
}

func OpenContextMenu(c *core.Context, id core.ID) {
	c.CloseAllPopups()
	c.OpenPopup(id, c.Mouse())
}

func BeginContextMenu(c *core.Context, id core.ID, r mathx.Rect) bool {
	if c.RightClicked(id, r) {
		OpenContextMenu(c, id)
	}
	return c.BeginPopup(id, 220)
}

func ContextMenu(c *core.Context, id core.ID) bool {
	return c.BeginPopup(id, 220)
}

func EndContextMenu(c *core.Context) {
	c.EndPopup()
}

func Tooltip(c *core.Context, id core.ID, text string) {
	TooltipAfter(c, id, text, 0.5)
}

func Confirm(c *core.Context, id core.ID, title, message string, buttons ...string) int {
	th := &c.Theme
	sz := c.Size()
	w := min(float32(480), sz.X-40)
	lines := int(c.TextSize(message).X/(w-4*th.Padding)) + 1
	h := th.RowHeight*float32(3+lines) + th.Padding*4
	r := mathx.Rect{X: (sz.X - w) / 2, Y: (sz.Y - h) / 3, W: w, H: h}
	if !c.BeginModal(id, r) {
		return -1
	}
	c.PushRawID(id)
	result := -1
	Label(c, title)
	LabelWrapped(c, message, true)
	row := mathx.Rect{X: r.X + th.Padding, Y: r.Y + r.H - th.RowHeight - th.Padding, W: r.W - 2*th.Padding, H: th.RowHeight}
	x := row.X + row.W
	for i := len(buttons) - 1; i >= 0; i-- {
		bw := c.TextSize(buttons[i]).X + 28
		x -= bw
		if ButtonRect(c, c.IDInt(i), mathx.Rect{X: x, Y: row.Y, W: bw, H: row.H}, buttons[i]) {
			result = i
		}
		x -= 6
	}
	if result < 0 && len(buttons) > 0 && c.Focus() == 0 {
		if c.KeyPressed(input.KeyEnter) || c.KeyPressed(input.KeyKPEnter) {
			result = 0
		}
		if c.KeyPressed(input.KeyEscape) {
			result = len(buttons) - 1
		}
	}
	c.PopID()
	c.EndModal()
	return result
}

func TooltipAfter(c *core.Context, id core.ID, text string, delay float64) {
	if text == "" || c.HoverDuration(id) < delay {
		return
	}
	th := &c.Theme
	m := c.Mouse()
	size := c.TextSize(text)
	r := mathx.Rect{X: m.X + 14, Y: m.Y + 18, W: size.X + 2*th.Padding, H: size.Y + th.Padding}
	sz := c.Size()
	if r.X+r.W > sz.X {
		r.X = max(0, sz.X-r.W)
	}
	if r.Y+r.H > sz.Y {
		r.Y = max(0, m.Y-r.H-4)
	}
	l := c.List()
	l.PushLayer(draw.LayerTop)
	c.Shadow(r, th.Radius.Popup)
	c.Box(r, th.Radius.Popup, th.PanelAlt, th.Border)
	l.Text(text, mathx.Vec2{X: r.X + th.Padding, Y: r.Y + th.Padding/2}, th.Text)
	l.PopLayer()
}
