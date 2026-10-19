package dock

import (
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

const (
	floatMinW  = 220
	floatMinH  = 140
	floatW     = 560
	floatH     = 380
	gripSize   = 12
	floatGrace = 80
)

var menuID = core.IDOf("dock.menu")

type Float struct {
	Panel string
	Rect  mathx.Rect
}

type menuState struct {
	panel    string
	floating bool
}

func (t *Tree) floatIndex(panel string) int {
	for i := range t.floats {
		if t.floats[i].Panel == panel {
			return i
		}
	}
	return -1
}

func (t *Tree) Floats() []Float {
	return t.floats
}

func (t *Tree) floatContent(i int) mathx.Rect {
	r := t.floats[i].Rect
	return mathx.Rect{X: r.X, Y: r.Y + t.tabH, W: r.W, H: max(0, r.H-t.tabH)}
}

func (t *Tree) clampFloat(r mathx.Rect) mathx.Rect {
	b := t.bounds
	if b.W <= 0 || b.H <= 0 {
		return r
	}
	r.W = mathx.Clamp(r.W, floatMinW, max(floatMinW, b.W))
	r.H = mathx.Clamp(r.H, floatMinH, max(floatMinH, b.H))
	r.X = mathx.Clamp(r.X, b.X-r.W+floatGrace, b.X+b.W-floatGrace)
	r.Y = mathx.Clamp(r.Y, b.Y, max(b.Y, b.Y+b.H-t.tabH))
	return r
}

func (t *Tree) floatAt(m mathx.Vec2) mathx.Rect {
	return t.clampFloat(mathx.Rect{X: m.X - 60, Y: m.Y - 12, W: floatW, H: floatH})
}

func (t *Tree) Float(panel string, at mathx.Vec2) {
	t.Undock(panel)
	t.floats = append(t.floats, Float{Panel: panel, Rect: t.floatAt(at)})
	t.changed = true
}

func (t *Tree) raise(i int) {
	if i < 0 || i >= len(t.floats)-1 {
		return
	}
	f := t.floats[i]
	t.floats = append(t.floats[:i], t.floats[i+1:]...)
	t.floats = append(t.floats, f)
	t.changed = true
}

func (t *Tree) dockBack(panel string) {
	t.DockPanel(panel, t.firstTabs(t.root), SideCenter)
}

func (t *Tree) close(panel string) {
	if t.OnClose != nil && t.OnClose(panel) {
		return
	}
	t.Undock(panel)
}

func (t *Tree) openMenu(ui *core.Context, panel string, floating bool) {
	t.menu = menuState{panel: panel, floating: floating}
	ui.CloseAllPopups()
	ui.OpenPopup(menuID, ui.Mouse())
}

func (t *Tree) drawMenu(ui *core.Context) {
	if !ui.BeginPopup(menuID, 180) {
		return
	}
	p := t.menu.panel
	label := "Maximize"
	if t.Maximized() == p {
		label = "Restore Layout"
	}
	if widgets.MenuItem(ui, label, "Shift+Space", true, false) {
		t.ToggleMaximize(p)
	}
	switch {
	case t.menu.floating:
		if widgets.MenuItem(ui, "Dock", "", true, false) {
			t.dockBack(p)
		}
	case t.Maximized() == "":
		if widgets.MenuItem(ui, "Float", "Shift+drag", true, false) {
			t.Float(p, ui.Mouse())
		}
	}
	if widgets.MenuItem(ui, "Close", "", true, false) {
		t.close(p)
	}
	ui.EndPopup()
}

func (t *Tree) drawFloats(ui *core.Context, fn func(string, mathx.Rect)) {
	th := &ui.Theme
	raise, closeAt, dockAt := "", "", ""
	for i := range t.floats {
		f := &t.floats[i]
		f.Rect = t.clampFloat(f.Rect)
		r := f.Rect
		id := core.IDOf("dock.float/" + f.Panel)
		ui.BeginOverlay(r, draw.LayerPopup)
		ui.Rect(r, th.Panel)
		bar := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: t.tabH}
		ui.Rect(bar, th.Header)
		cr := mathx.Rect{X: bar.X + bar.W - bar.H, Y: bar.Y, W: bar.H, H: bar.H}
		dr := mathx.Rect{X: cr.X - bar.H, Y: bar.Y, W: bar.H, H: bar.H}
		title := mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, dr.X-bar.X), H: bar.H}
		if ui.Clicked(core.IDSeq(id, 1), cr) {
			closeAt = f.Panel
		}
		if ui.Clicked(core.IDSeq(id, 2), dr) {
			dockAt = f.Panel
		}
		if d, ok := ui.Drag(id, title); ok && (d.X != 0 || d.Y != 0) {
			f.Rect.X += d.X
			f.Rect.Y += d.Y
			t.changed = true
		}
		if ui.DoubleClicked(id) {
			dockAt = f.Panel
		}
		if ui.RightClicked(id, title) {
			t.openMenu(ui, f.Panel, true)
		}
		ui.Text(t.title(f.Panel), mathx.Vec2{X: bar.X + th.Padding + 4, Y: ui.TextY(bar)}, th.Text)
		widgets.IconAt(ui, mathx.Rect{X: dr.X + 5, Y: dr.Y + 5, W: dr.W - 10, H: dr.H - 10}, "view.dock", th.TextDim)
		widgets.IconAt(ui, mathx.Rect{X: cr.X + 5, Y: cr.Y + 5, W: cr.W - 10, H: cr.H - 10}, "ui.close", th.TextDim)
		content := t.floatContent(i)
		if content.W > 0 && content.H > 0 {
			ui.PushClip(content)
			ui.PushID(f.Panel)
			fn(f.Panel, content)
			ui.PopID()
			ui.PopClip()
		}
		grip := mathx.Rect{X: r.X + r.W - gripSize, Y: r.Y + r.H - gripSize, W: gripSize, H: gripSize}
		gid := core.IDSeq(id, 3)
		if d, ok := ui.Drag(gid, grip); ok && (d.X != 0 || d.Y != 0) {
			f.Rect.W = max(floatMinW, f.Rect.W+d.X)
			f.Rect.H = max(floatMinH, f.Rect.H+d.Y)
			t.changed = true
		}
		gc := th.Border
		if ui.Hot() == gid || ui.Active() == gid {
			gc = th.Accent
			ui.SetCursorShape(core.CursorResizeNWSE)
		}
		for k := float32(0); k < 3; k++ {
			ui.Rect(mathx.Rect{X: grip.X + 3 + k*3, Y: grip.Y + gripSize - 3 - k*3, W: 2, H: 2}, gc)
		}
		ui.Outline(r, th.Border)
		ui.EndOverlay()
		if ui.MousePressed(input.MouseLeft) && r.Contains(ui.Mouse()) {
			raise = f.Panel
		}
	}
	switch {
	case closeAt != "":
		t.close(closeAt)
	case dockAt != "":
		t.dockBack(dockAt)
	case raise != "":
		t.raise(t.floatIndex(raise))
	}
}
