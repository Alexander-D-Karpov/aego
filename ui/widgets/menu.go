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
	return m.MenuEnabled(label, width, true)
}

// MenuEnabled draws a menu bar entry that cannot open while disabled.
func (m *MenuBar) MenuEnabled(label string, width float32, enabled bool) bool {
	c := m.c
	th := &c.Theme
	id := c.ID("menu/" + label)
	w := c.TextSize(label).X + 2*th.Padding + 4
	br := mathx.Rect{X: m.x, Y: m.r.Y, W: w, H: m.r.H}
	m.x += w
	if !enabled {
		c.ClosePopup(id)
		c.TextIn(br, label, th.TextDisabled, core.AlignCenter)
		return false
	}
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
		c.RoundRect(mathx.Rect{X: br.X, Y: br.Y + 3, W: br.W, H: br.H - 6}, th.Radius.Menu, th.ButtonHover)
	}
	c.TextIn(br, label, th.Text, core.AlignCenter)
	return c.BeginPopup(id, width)
}

// Button is a menu bar entry without a popup; at is where a window it opens should go.
func (m *MenuBar) Button(label string) (clicked bool, at mathx.Vec2) {
	c := m.c
	th := &c.Theme
	id := c.ID("menu/" + label)
	w := c.TextSize(label).X + 2*th.Padding + 4
	br := mathx.Rect{X: m.x, Y: m.r.Y, W: w, H: m.r.H}
	m.x += w
	clicked = c.Pressed(id, br)
	if clicked {
		c.CloseAllPopups()
	}
	if c.Hovered(id) {
		c.RoundRect(mathx.Rect{X: br.X, Y: br.Y + 3, W: br.W, H: br.H - 6}, th.Radius.Menu, th.ButtonHover)
	}
	c.TextIn(br, label, th.Text, core.AlignCenter)
	return clicked, mathx.Vec2{X: br.X, Y: br.Y + br.H}
}

func (m *MenuBar) EndMenu() {
	m.c.EndPopup()
}

func MenuItem(c *core.Context, label, shortcut string, enabled, checked bool) bool {
	return Entry(c, Item{Label: label, Shortcut: shortcut, Enabled: enabled, Checked: checked})
}

func MenuItemIcon(c *core.Context, label, shortcut, icon string, enabled, checked bool) bool {
	return Entry(c, Item{Label: label, Shortcut: shortcut, Icon: icon, Enabled: enabled, Checked: checked})
}

func SubMenu(c *core.Context, label string, width float32) bool {
	return SubMenuIcon(c, label, "", width)
}

func SubMenuIcon(c *core.Context, label, icon string, width float32) bool {
	th := &c.Theme
	r := c.Cursor().Next(0)
	c.FitPopup(26 + c.TextSize(label).X + r.H + 8)
	id := c.ID("sub/" + label)
	hot := c.MouseOver(id, r)
	open := c.PopupOpen(id)
	if hot && !open {
		c.CloseChildren()
		c.OpenPopup(id, mathx.Vec2{X: r.X + r.W + th.Padding, Y: r.Y - th.Padding})
		open = true
	}
	if hot || open {
		c.RoundRect(r, th.Radius.Menu, th.Selection)
	}
	if HasIcon(c, icon) {
		IconAt(c, mathx.Rect{X: r.X + 4, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, icon, th.TextDim)
	}
	c.Text(label, mathx.Vec2{X: r.X + 26, Y: c.TextY(r)}, th.Text)
	Chevron(c, mathx.Rect{X: r.X + r.W - r.H, Y: r.Y, W: r.H, H: r.H}, false, th.TextDim)
	return c.BeginPopup(id, width)
}

func EndSubMenu(c *core.Context) {
	c.EndPopup()
}

const tipDelay = 0.6

type Item struct {
	Label    string
	Shortcut string
	Icon     string
	Tip      string
	Enabled  bool
	Checked  bool
	KeepOpen bool
}

func HasIcon(c *core.Context, id string) bool {
	return id != "" && c.Icons != nil && c.Icons.Has(id)
}

func Entry(c *core.Context, it Item) bool {
	th := &c.Theme
	r := c.Cursor().Next(0)
	need := 26 + c.TextSize(it.Label).X + 8
	if it.Shortcut != "" {
		need += 24 + c.TextSize(it.Shortcut).X
	}
	c.FitPopup(need)
	id := c.ID("item/" + it.Label)
	clicked := false
	if it.Enabled {
		clicked = c.Clicked(id, r)
		if c.Hot() == id {
			c.CloseChildren()
			c.RoundRect(r, th.Radius.Menu, th.Selection)
		}
	} else {
		c.MouseOver(id, r)
	}
	col, tint := th.Text, th.TextDim
	if !it.Enabled {
		col, tint = th.TextDisabled, th.TextDisabled
	}
	box := mathx.Rect{X: r.X + 4, Y: r.Y + (r.H-16)/2, W: 16, H: 16}
	switch {
	case it.Checked:
		CheckMark(c, box, th.Accent)
	case HasIcon(c, it.Icon):
		IconAt(c, box, it.Icon, tint)
	}
	c.Text(it.Label, mathx.Vec2{X: r.X + 26, Y: c.TextY(r)}, col)
	if it.Shortcut != "" {
		c.TextIn(r, it.Shortcut, th.TextDim, core.AlignRight)
	}
	TooltipAfter(c, id, it.Tip, tipDelay)
	if clicked && !it.KeepOpen {
		c.CloseAllPopups()
	}
	return clicked
}
