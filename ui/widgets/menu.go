package widgets

import (
	"aego/core/mathx"
	"aego/ui/core"
)

type MenuBar struct {
	c *core.Context
	r mathx.Rect
	x float32
}

func BeginMenuBar(c *core.Context, r mathx.Rect) MenuBar {
	th := &c.Theme
	c.Rect(r, th.BarBg)
	c.Rect(mathx.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, th.BarBorder)
	return MenuBar{c: c, r: r, x: r.X + 4}
}

func (m *MenuBar) X() float32 {
	return m.x
}

func (m *MenuBar) Menu(label string, width float32) bool {
	c := m.c
	th := &c.Theme
	id := c.ID("menu/" + label)
	w := c.TextSize(label).X + 2*th.Padding + 4
	br := mathx.Rect{X: m.x, Y: m.r.Y, W: w, H: m.r.H}
	m.x += w
	open := c.PopupOpen(id)
	at := mathx.Vec2{X: br.X, Y: br.Y + br.H}
	if c.Pressed(id, br) {
		c.CloseAllPopups()
		if !open {
			c.OpenPopup(id, at)
		}
	} else if !open && c.AnyPopup() && c.Hovered(id) {
		c.CloseAllPopups()
		c.OpenPopup(id, at)
	}
	if c.PopupOpen(id) || c.Hovered(id) {
		c.Rect(br, th.ButtonHover)
	}
	c.TextIn(br, label, th.Text, core.AlignCenter)
	return c.BeginPopup(id, width)
}

func (m *MenuBar) EndMenu() {
	m.c.EndPopup()
}

func MenuItem(c *core.Context, label, shortcut string, enabled, checked bool) bool {
	th := &c.Theme
	r := c.Cursor().Next(0)
	need := 20 + c.TextSize(label).X + 8
	if shortcut != "" {
		need += 24 + c.TextSize(shortcut).X
	}
	c.FitPopup(need)
	id := c.ID("item/" + label)
	clicked := false
	if enabled {
		clicked = c.Clicked(id, r)
		if c.Hot() == id {
			c.CloseChildren()
			c.Rect(r, th.Selection)
		}
	}
	col := th.Text
	if !enabled {
		col = th.TextDisabled
	}
	if checked {
		CheckMark(c, mathx.Rect{X: r.X + 2, Y: r.Y + (r.H-14)/2, W: 14, H: 14}, th.Accent)
	}
	c.Text(label, mathx.Vec2{X: r.X + 20, Y: c.TextY(r)}, col)
	if shortcut != "" {
		c.TextIn(r, shortcut, th.TextDim, core.AlignRight)
	}
	if clicked {
		c.CloseAllPopups()
	}
	return clicked
}

func SubMenu(c *core.Context, label string, width float32) bool {
	th := &c.Theme
	r := c.Cursor().Next(0)
	c.FitPopup(20 + c.TextSize(label).X + r.H + 8)
	id := c.ID("sub/" + label)
	hot := c.MouseOver(id, r)
	open := c.PopupOpen(id)
	if hot && !open {
		c.CloseChildren()
		c.OpenPopup(id, mathx.Vec2{X: r.X + r.W + th.Padding, Y: r.Y - th.Padding})
		open = true
	}
	if hot || open {
		c.Rect(r, th.Selection)
	}
	c.Text(label, mathx.Vec2{X: r.X + 20, Y: c.TextY(r)}, th.Text)
	Chevron(c, mathx.Rect{X: r.X + r.W - r.H, Y: r.Y, W: r.H, H: r.H}, false, th.TextDim)
	return c.BeginPopup(id, width)
}

func EndSubMenu(c *core.Context) {
	c.EndPopup()
}
