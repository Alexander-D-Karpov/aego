package inspector

import (
	"slices"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

const CommandLock = "inspector.lock"

func (p *Panel) Locked() bool {
	return p.locked
}

func (p *Panel) unlock() {
	p.locked, p.lockNodes = false, nil
}

func (p *Panel) ToggleLock() {
	if p.locked {
		p.unlock()
		return
	}
	d := p.d.Docs.Doc()
	sel := p.d.Selection.Selected()
	if d == nil || len(sel) == 0 {
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.inspector.select_the_nodes_to_keep", "Select the nodes to keep in the Inspector"))
		return
	}
	p.locked, p.lockDoc, p.lockNodes = true, d.ID, slices.Clone(sel)
}

func (p *Panel) target() *document.SceneDocument {
	if !p.locked {
		return p.d.Docs.Doc()
	}
	if cur := p.d.Docs.Doc(); cur != nil && cur.ID == p.lockDoc {
		return cur
	}
	if d := p.d.Docs.Documents().Get(p.lockDoc); d != nil {
		return d
	}
	p.unlock()
	return p.d.Docs.Doc()
}

func (p *Panel) cur() *document.SceneDocument {
	if p.doc != nil {
		return p.doc
	}
	return p.d.Docs.Doc()
}

func (p *Panel) lockBar(d *document.SceneDocument, r mathx.Rect) mathx.Rect {
	ui := p.f.UI
	th := &ui.Theme
	bar := ui.Cursor().Next(0)
	icon, tip := "ui.unlock", i18n.Tr("editor.inspector.lock.tip", "Lock the Inspector to this selection")
	if p.locked {
		icon, tip = "ui.lock", i18n.Tr("editor.inspector.unlock.tip", "Unlock the Inspector")
		bg := th.Accent
		bg.A = 0.15
		ui.Rect(bar, bg)
		text := i18n.TrN("editor.inspector.locked_to", int64(len(p.nodes)), "Locked to {n} node in {doc}", "Locked to {n} nodes in {doc}", i18n.Str("doc", d.Label()))
		ui.PushClip(mathx.Rect{X: bar.X, Y: bar.Y, W: max(0, bar.W-bar.H-4), H: bar.H})
		ui.Text(text, mathx.Vec2{X: bar.X + 4, Y: ui.TextY(bar)}, th.Text)
		ui.PopClip()
	}
	id := ui.ID("lock")
	br := mathx.Rect{X: bar.X + bar.W - bar.H, Y: bar.Y, W: bar.H, H: bar.H}
	if ui.Clicked(id, br) {
		p.ToggleLock()
	}
	if ui.Hot() == id {
		ui.Rect(br, th.WidgetHot)
	}
	col := th.TextDim
	if p.locked {
		col = th.Accent
	}
	widgets.IconAt(ui, mathx.Rect{X: br.X + 3, Y: br.Y + 3, W: br.W - 6, H: br.H - 6}, icon, col)
	widgets.Tooltip(ui, id, tip)
	top := bar.Y + bar.H + th.Spacing
	return mathx.Rect{X: r.X, Y: top, W: r.W, H: max(0, r.Y+r.H-top)}
}
