package inspector

import (
	"strings"
	"unsafe"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/dataedit"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/widgets"
)

// NewAssetView shares the existing schema services and document journals while
// giving a document tab independent scroll/gesture state from the Inspector.
func (p *Panel) NewAssetView() *Panel { return New(p.reg, p.d) }

// DrawAsset renders one explicit data/registered asset without changing global
// selection or inspecting the active Scene document.
func (p *Panel) DrawAsset(f *services.Frame, r mathx.Rect, id ids.AssetID) {
	p.f, p.doc = f, nil
	p.assetBoxes = p.assetBoxes[:0]
	if p.d.AssetView != nil && p.d.AssetView(id) {
		return
	}
	if p.d.Data != nil {
		p.drawData(r, id)
	}
}

func (p *Panel) schemaOf(d *document.SceneDocument) *schema.Registry {
	if d != nil {
		return d.Scene().Registry()
	}
	return p.d.Schema
}

func (p *Panel) dataFits(kind string, id ids.AssetID) bool {
	name, ok := strings.CutPrefix(kind, "data:")
	if !ok {
		name, ok = strings.CutPrefix(kind, "table:")
	}
	if !ok || p.d.Data == nil {
		return true
	}
	t, known := p.d.Data.TypeOf(id)
	return !known || t == name || strings.HasSuffix(t, "."+name)
}

func (p *Panel) drawData(r mathx.Rect, id ids.AssetID) {
	ui := p.f.UI
	th := &ui.Theme
	svc := p.d.Data
	doc, err := svc.Doc(id)
	if err != nil {
		widgets.LabelWrapped(ui, err.Error(), true)
		return
	}
	play := svc.InPlay()
	var live unsafe.Pointer
	if play {
		if live, err = svc.LiveRecord(id); err != nil {
			widgets.LabelWrapped(ui, err.Error(), true)
			return
		}
	}
	p.dataMenuID = ui.ID("datamenu")
	if ui.MouseOver(ui.ID("datascroll"), r) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
		}
	}
	p.scroll = mathx.Clamp(p.scroll, 0, max(0, p.content-r.H+th.Padding*2))
	cur := ui.Cursor()
	top := cur.Y
	cur.Y -= p.scroll
	td := doc.Type
	ui.PushID("data/" + id.String())
	action := ""
	if doc.Dirty() && !play {
		action = i18n.Tr("editor.common.save", "Save")
	}
	c := p.beginCard("data:"+td.FullName, p.reg.Meta(td).DisplayName, i18n.Tr("editor.inspector.data", "Data"), "cat.node", action, false)
	if c.action {
		p.report(svc.SaveDoc(doc))
	}
	if c.open {
		p.dataHeader(doc, play)
		p.dataBase(doc, play)
		for _, s := range doc.Issues() {
			p.dataIssue(s)
		}
		category := ""
		for i := range td.Fields {
			fd := &td.Fields[i]
			if !visibleField(fd, play) {
				continue
			}
			if cat := fd.Hints.Category; cat != "" && cat != category {
				category = cat
				widgets.LabelDim(ui, cat)
			}
			p.dataField(doc, fd, play, live)
		}
	}
	p.endCard(ui, c)
	ui.PopID()
	p.content = cur.Y + p.scroll - top
	if widgets.ContextMenu(ui, p.dataMenuID) {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.inspector.reset_to_base_value", "Reset to Base Value"), "", "ui.reset", p.dataMenu != "", false) {
			p.report(doc.Revert(p.dataMenu))
		}
		widgets.EndContextMenu(ui)
	}
}

func (p *Panel) dataHeader(doc *dataedit.Doc, play bool) {
	ui := p.f.UI
	widgets.LabelDim(ui, doc.Path)
	if play {
		widgets.LabelDim(ui, i18n.Tr("editor.inspector.play_live_fields_apply_now", "Play: live fields apply now, Stop asks whether to keep them"))
		return
	}
	row := ui.Cursor().Next(0)
	w := (row.W - 8) / 3
	if widgets.ButtonRect(ui, ui.ID("undo"), mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.inspector.undo", "Undo")) {
		p.report(doc.Undo())
	}
	if widgets.ButtonRect(ui, ui.ID("redo"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.inspector.redo", "Redo")) {
		p.report(doc.Redo())
	}
	if widgets.ButtonRect(ui, ui.ID("revert"), mathx.Rect{X: row.X + 2*(w+4), Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.inspector.revert", "Revert")) {
		p.d.Data.RevertDoc(doc.ID)
	}
}

func (p *Panel) dataBase(doc *dataedit.Doc, play bool) {
	ui := p.f.UI
	fd := &schema.FieldDescriptor{Name: "base", Kind: schema.KindAssetRef, AssetKind: "data:" + shortScript(doc.Type.FullName), Flags: schema.FlagEditor}
	v := schema.Asset(doc.Base())
	ui.PushID("base")
	defer ui.PopID()
	e := p.value(nil, fd, &v, false, i18n.Tr("editor.inspector.base", "Base"), i18n.Tr("editor.inspector.base.tip", "Fields this asset does not set come from its base"), play)
	if e.Changed && !play {
		p.report(doc.SetBase(v.Asset))
	}
}

func (p *Panel) dataIssue(s string) {
	ui := p.f.UI
	th := &ui.Theme
	r := ui.Cursor().Next(0)
	widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "status.warning", th.Warn)
	ui.PushClip(r)
	ui.Text(s, mathx.Vec2{X: r.X + 20, Y: ui.TextY(r)}, th.TextDim)
	ui.PopClip()
}

func (p *Panel) dataField(doc *dataedit.Doc, fd *schema.FieldDescriptor, play bool, live unsafe.Pointer) {
	ui := p.f.UI
	ui.PushRawID(core.ID(fd.ID))
	defer ui.PopID()
	src := doc.Ptr()
	if play {
		src = live
	}
	var v schema.Value
	fd.Get(src, &v)
	isLive := fd.Flags.Has(schema.FlagLive)
	ro := fd.Flags.Has(schema.FlagReadOnly) || (play && !isLive)
	tip := fd.Hints.Tooltip
	if isLive {
		tip = strings.TrimSpace(tip + "\nLive: can be tuned during Play")
	}
	cur := ui.Cursor()
	top := cur.Y
	e := p.value(nil, fd, &v, false, p.labelOf(nil, fd), tip, ro)
	row := mathx.Rect{X: cur.X, Y: top, W: cur.W, H: max(0, cur.Y-top)}
	over := !play && doc.Overridden(fd.Name)
	if over {
		ui.Rect(mathx.Rect{X: row.X - 5, Y: row.Y + 1, W: 2, H: max(row.H-2, 2)}, ui.Theme.Accent)
		if ui.Hit(row) && !ui.MouseInOverlay() && ui.MousePressed(input.MouseRight) {
			p.dataMenu = fd.Name
			widgets.OpenContextMenu(ui, p.dataMenuID)
		}
	}
	if e.Begin {
		p.dataStep = true
	}
	if !e.Changed {
		if e.End {
			p.dataStep = false
		}
		return
	}
	var err error
	if play {
		err = p.d.Data.LiveSet(doc.ID, fd, v)
	} else {
		err = doc.Set(fd, v, p.dataStep)
	}
	p.dataStep = false
	p.report(err)
}
