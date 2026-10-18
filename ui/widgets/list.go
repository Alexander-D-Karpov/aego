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
		if w := c.ScrollY(rowH * 3); w != 0 {
			*scroll -= w
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
	c.RoundRect(thumb, (track.W-2)/2, col)
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
	Icon        string
	Marker      bool
	Guides      bool
	Last        bool
	Lines       uint64
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
	if row.Guides && row.Depth > 0 {
		treeGuides(c, r, x, row)
	}
	if row.HasChildren {
		col := th.TextDim
		if c.Hot() == arrowID {
			col = th.Text
		}
		Chevron(c, arrow, row.Open, col)
	}
	col, tint := th.Text, th.TextDim
	if row.Dim {
		col, tint = th.TextDisabled, th.TextDisabled
	}
	lx := x + th.Indent + 4
	ir := mathx.Rect{X: lx, Y: r.Y + (r.H-14)/2, W: 14, H: 14}
	switch {
	case HasIcon(c, row.Icon):
		IconAt(c, ir, row.Icon, tint)
		lx += 18
	case row.Marker:
		c.RoundRect(mathx.Rect{X: ir.X + 4, Y: ir.Y + 4, W: 6, H: 6}, 3, th.TextDisabled)
		lx += 18
	}
	c.Text(row.Label, mathx.Vec2{X: lx, Y: c.TextY(r)}, col)
	return ev
}

func treeGuides(c *core.Context, r mathx.Rect, x float32, row TreeRow) {
	th := &c.Theme
	col := th.TreeGuide
	mid := float32(int(r.Y + r.H/2))
	for d := 0; d < row.Depth-1 && d < 64; d++ {
		if row.Lines&(1<<d) != 0 {
			c.Rect(mathx.Rect{X: float32(int(r.X + float32(d)*th.Indent + th.Indent/2)), Y: r.Y, W: 1, H: r.H}, col)
		}
	}
	gx := float32(int(r.X + float32(row.Depth-1)*th.Indent + th.Indent/2))
	h := r.H
	if row.Last {
		h = mid - r.Y + 1
	}
	c.Rect(mathx.Rect{X: gx, Y: r.Y, W: 1, H: h}, col)
	end := x + th.Indent
	if row.HasChildren {
		end = x + 2
	}
	c.Rect(mathx.Rect{X: gx + 1, Y: mid, W: max(0, end-gx-1), H: 1}, col)
}

type ScrollState struct {
	grab float32
}

func ScrollBar(c *core.Context, id core.ID, track mathx.Rect, scroll *float32, content, view float32, st *ScrollState) {
	th := &c.Theme
	maxOff := max(0, content-view)
	if maxOff <= 0 || track.H <= 0 {
		*scroll = 0
		return
	}
	c.RoundRect(track, track.W/2, th.PanelAlt)
	thumbH := min(track.H, max(24, track.H*view/content))
	span := track.H - thumbH
	ty := track.Y + span*(*scroll/maxOff)
	thumb := mathx.Rect{X: track.X + 1, Y: ty, W: track.W - 2, H: thumbH}
	if c.Pressed(id, track) {
		my := c.Mouse().Y
		if my >= thumb.Y && my < thumb.Y+thumb.H {
			st.grab = my - thumb.Y
		} else {
			st.grab = thumbH / 2
		}
		c.CaptureMouse(id)
	}
	if c.Active() == id {
		if c.MouseDown(input.MouseLeft) {
			if span > 0 {
				*scroll = mathx.Clamp((c.Mouse().Y-track.Y-st.grab)/span*maxOff, 0, maxOff)
			}
		} else {
			c.ReleaseMouse()
		}
		ty = track.Y + span*(*scroll/maxOff)
		thumb.Y = ty
	}
	col := th.Widget
	if c.Hot() == id || c.Active() == id {
		col = th.WidgetHot
	}
	c.RoundRect(thumb, (track.W-2)/2, col)
}
