package sceneview

import (
	"aego/core/mathx"
	"aego/editor/core/document"
	"aego/platform/input"
	"aego/ui/widgets"
)

const (
	backLabel = "‹ Back"
	crumbSep  = " › "
)

func (p *Panel) prefabBar(d *document.SceneDocument, r mathx.Rect) {
	docs := p.docs()
	if docs.InPlay() {
		return
	}
	mode := d.IsPrefab()
	var crumbs []*document.SceneDocument
	switch {
	case mode:
		crumbs = docs.Chain(d.ID)
	default:
		pending := docs.Pending(d.ID)
		if len(pending) == 0 {
			return
		}
		crumbs = append([]*document.SceneDocument{d}, pending...)
	}
	ui := p.ui()
	th := &ui.Theme
	if mode {
		ui.Outline(r, th.Accent)
		ui.Outline(mathx.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}, th.Accent)
		if p.f.Focused && len(p.d.Selection.Selected()) == 0 && ui.KeyPressed(input.KeyEscape) {
			p.exitPrefab(d)
			return
		}
	}
	_, canBack := docs.BackTarget(d.ID)
	canBack = canBack && mode
	h := th.BarHeight
	bw := ui.TextSize(backLabel).X + 16
	w := float32(12)
	if canBack {
		w += bw + 8
	}
	for i, c := range crumbs {
		w += ui.TextSize(c.Caption()).X
		if i < len(crumbs)-1 {
			w += ui.TextSize(crumbSep).X
		}
	}
	bar := mathx.Rect{X: r.X + 6, Y: p.barRect.Y + p.barRect.H + 4, W: min(w, r.W-12), H: h}
	ui.Rect(bar, th.Header)
	ui.Outline(bar, th.Accent)
	ui.PushClip(bar)
	defer ui.PopClip()

	x := bar.X + 6
	if canBack {
		if widgets.ButtonRect(ui, ui.ID("prefab.back"), mathx.Rect{X: x, Y: bar.Y + 3, W: bw, H: h - 6}, backLabel) {
			p.exitPrefab(d)
			return
		}
		x += bw + 8
	}
	for i, c := range crumbs {
		label := c.Caption()
		cw := ui.TextSize(label).X
		cr := mathx.Rect{X: x, Y: bar.Y, W: cw, H: h}
		col := th.Text
		if c.ID != d.ID {
			id := ui.IDInt(900 + i)
			col = th.TextDim
			if ui.Hot() == id {
				col = th.Accent
			}
			if ui.Pressed(id, cr) {
				docs.SetActive(c.ID)
			}
			widgets.Tooltip(ui, id, c.Path)
		}
		ui.Text(label, mathx.Vec2{X: x, Y: ui.TextY(cr)}, col)
		x += cw
		if i < len(crumbs)-1 {
			ui.Text(crumbSep, mathx.Vec2{X: x, Y: ui.TextY(cr)}, th.TextDim)
			x += ui.TextSize(crumbSep).X
		}
	}
}

func (p *Panel) exitPrefab(d *document.SceneDocument) {
	docs := p.docs()
	if _, ok := docs.BackTarget(d.ID); !ok {
		return
	}
	if d.Dirty() {
		docs.Back(d.ID)
		return
	}
	if docs.Close(d.ID, false) != nil {
		docs.Back(d.ID)
	}
}
