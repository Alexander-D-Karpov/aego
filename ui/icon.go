package ui

import (
	"aego/core/mathx"
	"aego/ui/core"
	"aego/ui/widgets"
)

func Icon(c *core.Context, id string, size float32) {
	r := c.Cursor().Next(size)
	widgets.IconAt(c, mathx.Rect{X: r.X, Y: r.Y, W: size, H: size}, id, c.Theme.Text)
}

func IconButton(c *core.Context, uid core.ID, id string, size float32, tooltip string) bool {
	th := &c.Theme
	r := c.Cursor().Next(size + 6)
	br := mathx.Rect{X: r.X, Y: r.Y, W: size + 6, H: size + 6}
	clicked := c.Clicked(uid, br)
	if c.Hot() == uid || c.Active() == uid {
		c.Rect(br, th.ButtonHover)
	}
	widgets.IconAt(c, mathx.Rect{X: br.X + 3, Y: br.Y + 3, W: size, H: size}, id, th.Text)
	widgets.Tooltip(c, uid, tooltip)
	return clicked
}
