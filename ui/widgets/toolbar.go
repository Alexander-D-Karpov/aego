package widgets

import (
	"math"
	"strings"

	"aego/core/mathx"
	"aego/ui/core"
)

type DotState uint8

const (
	DotIdle DotState = iota
	DotBusy
	DotOK
	DotError
)

func IconAt(c *core.Context, r mathx.Rect, id string, tint mathx.Color) {
	r = snap(r, c.Scale())
	if c.Icons != nil {
		if reg, ok := c.Icons.Region(id, r.H, c.Scale()); ok {
			c.List().Texture(reg, r, tint)
			return
		}
	}
	th := &c.Theme
	c.Rect(r, th.Badge)
	name := id
	if i := strings.LastIndexByte(id, '.'); i >= 0 {
		name = id[i+1:]
	}
	if name != "" {
		letter := name[:1]
		if b := name[0]; b >= 'a' && b <= 'z' {
			const upper = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
			letter = upper[b-'a' : b-'a'+1]
		}
		c.TextIn(r, letter, th.TextDim, core.AlignCenter)
	}
}

type Toolbar struct {
	c      *core.Context
	r      mathx.Rect
	x      float32
	Active mathx.Color
	Label  mathx.Color
}

func BeginToolbar(c *core.Context, r mathx.Rect) Toolbar {
	th := &c.Theme
	c.Rect(r, th.BarBg)
	c.Rect(mathx.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, th.BarBorder)
	return Toolbar{c: c, r: r, x: r.X + 6}
}

func BeginToolbarIn(c *core.Context, r mathx.Rect) Toolbar {
	return Toolbar{c: c, r: r, x: r.X + 6}
}

func (t *Toolbar) X() float32 {
	return t.x
}

func (t *Toolbar) SetX(x float32) {
	t.x = x
}

func (t *Toolbar) ButtonW() float32 {
	return t.r.H - 6 + 2
}

func (t *Toolbar) Button(id core.ID, icon, tooltip string, active, enabled bool) bool {
	return t.TintButton(id, icon, tooltip, active, enabled, t.c.Theme.Text)
}

func (t *Toolbar) TintButton(id core.ID, icon, tooltip string, active, enabled bool, tint mathx.Color) bool {
	c := t.c
	th := &c.Theme
	s := t.r.H - 6
	br := mathx.Rect{X: t.x, Y: t.r.Y + 3, W: s, H: s}
	t.x += s + 2
	clicked := false
	if enabled {
		clicked = c.Clicked(id, br)
	} else {
		c.MouseOver(id, br)
	}
	switch {
	case active:
		bg := th.ButtonActive
		if t.Active.A > 0 {
			bg = t.Active
		}
		c.RoundRect(br, th.Radius.Button, bg)
	case enabled && (c.Hot() == id || c.Active() == id):
		c.RoundRect(br, th.Radius.Button, th.ButtonHover)
	}
	if !enabled {
		tint = th.TextDisabled
	}
	is := min(float32(16), s-4)
	ix, iy := float32(int(br.X+(s-is)/2)), float32(int(br.Y+(s-is)/2))
	IconAt(c, mathx.Rect{X: ix, Y: iy, W: is, H: is}, icon, tint)
	Tooltip(c, id, tooltip)
	return clicked
}

func (t *Toolbar) TextButton(id core.ID, label, tooltip string) (bool, mathx.Rect) {
	c := t.c
	th := &c.Theme
	w := c.TextSize(label).X + 14
	br := mathx.Rect{X: t.x, Y: t.r.Y + 3, W: w, H: t.r.H - 6}
	t.x += w + 2
	clicked := c.Clicked(id, br)
	if c.Hot() == id || c.Active() == id {
		c.RoundRect(br, th.Radius.Button, th.ButtonHover)
	}
	col := th.TextDim
	if t.Label.A > 0 {
		col = t.Label
	}
	c.TextIn(br, label, col, core.AlignCenter)
	Tooltip(c, id, tooltip)
	return clicked, br
}

func (t *Toolbar) Separator() {
	t.x += 4
	t.c.Rect(mathx.Rect{X: t.x, Y: t.r.Y + 6, W: 1, H: t.r.H - 12}, t.c.Theme.BarBorder)
	t.x += 5
}

func StatusDot(c *core.Context, id core.ID, r mathx.Rect, state DotState, label string) bool {
	th := &c.Theme
	clicked := c.Clicked(id, r)
	if c.Hot() == id {
		c.Rect(r, th.ButtonHover)
	}
	col := th.TextDisabled
	switch state {
	case DotBusy:
		col = th.Warn
	case DotOK:
		col = th.Ok
	case DotError:
		col = th.Error
	}
	d := mathx.Rect{X: r.X + 6, Y: r.Y + (r.H-8)/2, W: 8, H: 8}
	c.Rect(d, col)
	c.Text(label, mathx.Vec2{X: d.X + d.W + 6, Y: c.TextY(r)}, th.TextDim)
	return clicked
}

func IconButton(c *core.Context, id core.ID, r mathx.Rect, icon, tooltip string) bool {
	th := &c.Theme
	clicked := c.Clicked(id, r)
	tint := th.TextDim
	if c.Hot() == id || c.Active() == id {
		c.RoundRect(r, th.Radius.Button, th.ButtonHover)
		tint = th.Text
	}
	IconIn(c, r, icon, tint)
	Tooltip(c, id, tooltip)
	return clicked
}

func IconIn(c *core.Context, r mathx.Rect, icon string, tint mathx.Color) {
	is := min(float32(16), min(r.W, r.H)-4)
	ix, iy := float32(int(r.X+(r.W-is)/2)), float32(int(r.Y+(r.H-is)/2))
	IconAt(c, mathx.Rect{X: ix, Y: iy, W: is, H: is}, icon, tint)
}

func snap(r mathx.Rect, scale float32) mathx.Rect {
	if scale <= 0 {
		scale = 1
	}
	round := func(v float32) float32 { return float32(math.Round(float64(v*scale))) / scale }
	x, y := round(r.X), round(r.Y)
	return mathx.Rect{X: x, Y: y, W: round(r.X+r.W) - x, H: round(r.Y+r.H) - y}
}
