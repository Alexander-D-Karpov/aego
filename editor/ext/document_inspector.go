package ext

import (
	"path"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

// DocumentView adapts an asset editor to a centre tab; the factory owns only view state, data/history stay in AssetDocument or its bound editor, and nil uses registered asset views.
func (r *Registry) DocumentView(factory func(*document.AssetDocument) func(*services.Frame, mathx.Rect)) func(*document.AssetDocument) error {
	return r.DocumentPanelFactory(func(d *document.AssetDocument) workspace.Panel {
		p := &assetDocumentPanel{reg: r, doc: d}
		if factory != nil {
			p.draw = factory(d)
		}
		return p
	})
}

// DocumentInspector reuses the module's registered asset inspector as a real
// document view. Opening it does not change the global Inspector selection.
func (r *Registry) DocumentInspector() func(*document.AssetDocument) error {
	return r.DocumentView(nil)
}

type assetDocumentPanel struct {
	reg  *Registry
	doc  *document.AssetDocument
	draw func(*services.Frame, mathx.Rect)
}

func (p *assetDocumentPanel) ID() string                        { return "asset.document/" + p.doc.Asset.String() }
func (p *assetDocumentPanel) Title() string                     { return path.Base(p.doc.Path) }
func (p *assetDocumentPanel) Icon() string                      { return p.doc.Icon }
func (p *assetDocumentPanel) Scope() string                     { return p.doc.Handler }
func (p *assetDocumentPanel) Dirty() bool                       { return p.doc.Dirty() }
func (p *assetDocumentPanel) Document() *document.AssetDocument { return p.doc }

func (p *assetDocumentPanel) Draw(f *services.Frame, area mathx.Rect) {
	ed, d, ui := p.reg.Editor(), p.doc, f.UI
	ui.PushID(p.ID())
	defer ui.PopID()
	ui.PushClip(area)
	defer ui.PopClip()
	ui.Region(area)
	defer ui.PopCursor()
	widgets.LabelDim(ui, d.Path)
	if d.Note != "" {
		widgets.LabelWrapped(ui, d.Note, true)
	}
	if d.Capabilities.Has(document.EditSource) || d.Capabilities.Has(document.EditMetadata) {
		row := ui.Cursor().Next(0)
		w := min(float32(100), (row.W-8)/3)
		if widgets.ButtonRect(ui, ui.ID("document-save"), mathx.Rect{X: row.X, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.common.save", "Save")) {
			p.report(ed.SaveAsset(d))
		}
		if widgets.ButtonRect(ui, ui.ID("undo"), mathx.Rect{X: row.X + w + 4, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.inspector.undo", "Undo")) {
			p.report(ed.UndoAsset(d))
		}
		if widgets.ButtonRect(ui, ui.ID("redo"), mathx.Rect{X: row.X + 2*w + 8, Y: row.Y, W: w, H: row.H}, i18n.Tr("editor.inspector.redo", "Redo")) {
			p.report(ed.RedoAsset(d))
		}
	}
	cur := ui.Cursor()
	body := mathx.Rect{X: cur.X, Y: cur.Y, W: cur.W, H: max(0, area.Y+area.H-cur.Y)}
	ui.PushClip(body)
	defer ui.PopClip()
	if p.draw != nil {
		p.draw(f, body)
		return
	}
	if draw := ed.AssetViews[string(d.Kind)]; draw != nil {
		draw(ed, d.Selected)
		return
	}
	widgets.LabelDim(ui, string(d.Kind)+" · "+d.Asset.String())
	widgets.LabelDim(ui, strings.Join(d.Capabilities.Names(), " · "))
	widgets.LabelWrapped(ui, i18n.Tr("editor.document.readonly_fallback", "Read-only asset information. This format has no specialized viewer in the enabled modules."), true)
	if d.Capabilities.Has(document.RevealSource) && widgets.Button(ui, i18n.Tr("editor.document.reveal_source", "Reveal Source")) && ed.RevealPath != nil {
		ed.RevealPath(d.Path)
	}
}

func (p *assetDocumentPanel) report(err error) {
	if err != nil {
		p.reg.Editor().Notify(context.LevelError, err.Error())
	}
}

func (p *assetDocumentPanel) RequestClose() {
	if !p.doc.Dirty() {
		p.close(false)
		return
	}
	p.reg.Editor().Ask(context.Question{
		Title:   i18n.Tr("editor.document.unsaved_title", "Unsaved asset changes"),
		Message: i18n.Tr("editor.document.unsaved_message", "Save changes to {path} before closing?", i18n.Str("path", p.doc.Path)),
		Buttons: []string{i18n.Tr("editor.common.save", "Save"), i18n.Tr("editor.image.discard", "Discard changes"), i18n.Tr("editor.image.cancel", "Cancel")},
		Then: func(choice int) {
			switch choice {
			case 0:
				if err := p.reg.Editor().SaveAsset(p.doc); err != nil {
					p.report(err)
					return
				}
				p.close(false)
			case 1:
				p.close(true)
			}
		},
	})
}

func (p *assetDocumentPanel) close(discard bool) {
	ed := p.reg.Editor()
	if err := ed.Docs.CloseAsset(p.doc.Asset, discard); err != nil {
		p.report(err)
		return
	}
	if player, ok := ed.AssetPlayers[string(p.doc.Kind)]; ok && player.Playing != nil && player.Stop != nil && player.Playing(ed) == p.doc.Asset {
		player.Stop(ed)
	}
	p.reg.ClosePanel(p.ID())
}
