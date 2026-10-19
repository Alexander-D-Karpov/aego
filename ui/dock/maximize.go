package dock

import (
	"aego/core/mathx"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/widgets"
)

func (t *Tree) Maximized() string {
	if t.max != "" && !t.Has(t.max) {
		t.max = ""
	}
	return t.max
}

func (t *Tree) Maximize(panel string) bool {
	if !t.Has(panel) {
		return false
	}
	t.max = panel
	return true
}

func (t *Tree) Restore() {
	t.max = ""
}

func (t *Tree) ToggleMaximize(panel string) bool {
	if t.Maximized() == panel {
		t.max = ""
		return false
	}
	return t.Maximize(panel)
}

func (t *Tree) drawMax(ui *core.Context, r mathx.Rect, panel string, fn func(string, mathx.Rect)) {
	th := &ui.Theme
	bar := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: t.tabH}
	ui.Rect(bar, th.Header)
	id := tabID(panel)
	tr := mathx.Rect{X: bar.X, Y: bar.Y, W: t.tabWidth(ui, panel), H: bar.H}
	if ui.Pressed(id, tr) && ui.DoubleClicked(id) {
		t.max = ""
	}
	if ui.RightClicked(id, tr) {
		t.openMenu(ui, panel, t.floatIndex(panel) >= 0)
	}
	ui.List().RoundRect(mathx.Rect{X: tr.X, Y: tr.Y + 3, W: tr.W, H: tr.H - 3}, th.Radius.Tab, th.TabActive, draw.CornersTop)
	ui.Rect(mathx.Rect{X: tr.X, Y: tr.Y + tr.H - 2, W: tr.W, H: 2}, th.Accent)
	t.tabLabel(ui, panel, tr, th.Text)
	rb := mathx.Rect{X: bar.X + bar.W - bar.H, Y: bar.Y, W: bar.H, H: bar.H}
	rid := core.IDSeq(id, 7)
	if ui.Clicked(rid, rb) {
		t.max = ""
	}
	col := th.TextDim
	if ui.Hot() == rid {
		col = th.Text
	}
	widgets.IconAt(ui, mathx.Rect{X: rb.X + 5, Y: rb.Y + 5, W: rb.W - 10, H: rb.H - 10}, "view.dock", col)
	widgets.Tooltip(ui, rid, "Restore the layout (Shift+Space)")
	content := mathx.Rect{X: r.X, Y: r.Y + t.tabH, W: r.W, H: max(0, r.H-t.tabH)}
	t.maxRect = content
	ui.Rect(content, th.Panel)
	if content.W <= 0 || content.H <= 0 {
		return
	}
	ui.PushClip(content)
	ui.PushID(panel)
	fn(panel, content)
	ui.PopID()
	ui.PopClip()
}
