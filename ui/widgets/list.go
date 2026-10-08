package widgets

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

func VirtualList(c *core.Context, id core.ID, r mathx.Rect, count int, rowH float32, scroll *float32, fn func(i int, row mathx.Rect)) {
	th := &c.Theme
	content := float32(count) * rowH
	maxOff := max(0, content-r.H)
	if c.MouseOver(id, r) {
		if w := c.WheelY(); w != 0 {
			*scroll -= w * rowH * 3
		}
	}
	*scroll = mathx.Clamp(*scroll, 0, maxOff)
	inner := r
	if content > r.H {
		inner.W -= th.ScrollbarW
	}
	c.PushClip(inner)
	if rowH > 0 && count > 0 {
		first := int(*scroll / rowH)
		last := min(count, int((*scroll+r.H)/rowH)+1)
		for i := first; i < last; i++ {
			fn(i, mathx.Rect{X: inner.X, Y: inner.Y + float32(i)*rowH - *scroll, W: inner.W, H: rowH})
		}
	}
	c.PopClip()
	if content <= r.H {
		return
	}
	track := mathx.Rect{X: inner.X + inner.W, Y: r.Y, W: th.ScrollbarW, H: r.H}
	c.Rect(track, th.PanelAlt)
	thumbH := max(20, r.H*r.H/content)
	ty := r.Y + (*scroll/maxOff)*(r.H-thumbH)
	thumb := mathx.Rect{X: track.X + 1, Y: ty, W: track.W - 2, H: thumbH}
	tid := core.IDSeq(id, 1)
	if d, ok := c.Drag(tid, thumb); ok && d.Y != 0 && r.H > thumbH {
		*scroll = mathx.Clamp(*scroll+d.Y*maxOff/(r.H-thumbH), 0, maxOff)
	}
	col := th.Widget
	if c.Hot() == tid || c.Active() == tid {
		col = th.WidgetHot
	}
	c.Rect(thumb, col)
}

func EnsureVisible(scroll *float32, index int, rowH, viewH float32) {
	top := float32(index) * rowH
	if top < *scroll {
		*scroll = top
	}
	if top+rowH > *scroll+viewH {
		*scroll = top + rowH - viewH
	}
}

type TreeRow struct {
	Depth       int
	Label       string
	HasChildren bool
	Open        bool
	Selected    bool
	Focused     bool
	Dim         bool
}

type RowEvent struct {
	Pressed       bool
	Released      bool
	DoubleClicked bool
	Toggled       bool
	RightClicked  bool
}

func TreeRowRect(c *core.Context, id core.ID, r mathx.Rect, row TreeRow) RowEvent {
	th := &c.Theme
	var ev RowEvent
	x := r.X + float32(row.Depth)*th.Indent
	arrow := mathx.Rect{X: x, Y: r.Y, W: th.Indent + 2, H: r.H}
	arrowID := id ^ 0x5bd1e995
	if row.HasChildren && c.Pressed(arrowID, arrow) {
		ev.Toggled = true
	} else if c.Pressed(id, r) {
		ev.Pressed = true
		ev.DoubleClicked = c.DoubleClicked(id)
	}
	if c.Active() == id && c.MouseReleased(input.MouseLeft) && c.Hit(r) {
		ev.Released = true
	}
	if c.RightClicked(id, r) {
		ev.RightClicked = true
	}
	switch {
	case row.Selected && row.Focused:
		c.Rect(r, th.Selection)
	case row.Selected:
		c.Rect(r, th.SelectionDim)
	case c.Hot() == id:
		c.Rect(r, th.WidgetHot)
	}
	if row.HasChildren {
		col := th.TextDim
		if c.Hot() == arrowID {
			col = th.Text
		}
		Chevron(c, arrow, row.Open, col)
	}
	col := th.Text
	if row.Dim {
		col = th.TextDisabled
	}
	c.Text(row.Label, mathx.Vec2{X: x + th.Indent + 4, Y: c.TextY(r)}, col)
	return ev
}
