package shell

import (
	"fmt"
	"path"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/preview"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/panels/animation"
	"aego/editor/panels/animator"
	"aego/engine/i18n"
	"aego/ui/widgets"
)

// Core routes adapt existing editors; modules add handlers through the extension registry, so the Asset Browser need not enumerate types or choose editors.
func (a *App) registerDocumentHandlers() error {
	if err := animation.RegisterThumbnails(a.ext); err != nil {
		return err
	}
	a.ctx.FallbackAsset = a.ext.DocumentInspector()
	openScene := func(d *document.AssetDocument) error {
		s, err := a.ctx.Docs.Open(d.Path)
		if err != nil {
			return err
		}
		d.Scene, d.ID = s, s.ID
		a.ws.Focus(workspace.Scene)
		return nil
	}
	openCode := func(d *document.AssetDocument) error {
		if d.Source == nil {
			s, err := a.ctx.Docs.OpenScript(d.Path)
			if err != nil {
				return err
			}
			d.Source = s
		}
		if a.ctx.Code == nil {
			return fmt.Errorf("the source editor is unavailable")
		}
		a.ctx.Code.Goto(context.At(d.Path, 1, 1))
		return nil
	}
	for _, h := range []document.Handler{
		{ID: "editor.authoring-source", Name: i18n.Text{En: "Project source"}, Icon: "file.script", Kinds: []asset.ArtifactKind{preview.SourceKind}, Extensions: []string{".go", ".aego"}, Capabilities: document.View | document.RevealSource, TextSource: true, Open: openCode},
		{ID: "editor.scene", Name: i18n.T("editor.document.scene", "Scene"), Icon: "file.scene", Kinds: []asset.ArtifactKind{asset.ArtScene}, Extensions: []string{".aescene"}, Capabilities: document.View | document.EditSource | document.Preview | document.RevealSource, Open: openScene},
		{ID: "editor.prefab", Name: i18n.T("editor.document.prefab", "Prefab"), Icon: "file.prefab", Kinds: []asset.ArtifactKind{asset.ArtPrefab}, Extensions: []string{".aeprefab"}, Capabilities: document.View | document.EditSource | document.Preview | document.RevealSource, Open: openScene},
		{ID: "editor.script", Name: i18n.T("editor.document.script", "Script"), Icon: "file.script", Kinds: []asset.ArtifactKind{asset.ArtScript}, Extensions: []string{".aes"}, Capabilities: document.View | document.EditSource | document.RevealSource, TextSource: true, Open: openCode},
		{ID: "editor.data", Name: i18n.T("editor.document.data", "Data"), Icon: "file.data", Kinds: []asset.ArtifactKind{asset.ArtData}, Extensions: []string{".aedata"}, Capabilities: document.View | document.EditSource | document.RevealSource, Open: a.openDataDocument},
		{ID: "editor.table", Name: i18n.T("editor.document.table", "Table"), Icon: "file.table", Kinds: []asset.ArtifactKind{asset.ArtTable}, Extensions: []string{".aetable"}, Capabilities: document.View | document.EditSource | document.RevealSource, Open: a.openTableDocument},
		{ID: "editor.animation", Name: i18n.T("editor.document.animation", "Animation"), Icon: "anim.key", Kinds: []asset.ArtifactKind{asset.ArtAnim}, Extensions: []string{".aeanim"}, Capabilities: document.View | document.EditSource | document.RevealSource, Open: func(d *document.AssetDocument) error {
			if (d.Source == nil || !d.Source.Dirty()) && a.animPanel != nil && a.animPanel.PickClip(d.Path) {
				a.ws.Show(animation.ID)
				return nil
			}
			d.Note = i18n.Tr("editor.document.animation_source", "Source view. The Timeline is available when this clip is bound to the selected node.")
			return openCode(d)
		}},
		{ID: "editor.controller", Name: i18n.T("editor.document.controller", "Animation Controller"), Icon: "anim.graph", Kinds: []asset.ArtifactKind{asset.ArtCtrl}, Extensions: []string{".aectrl"}, Capabilities: document.View | document.EditSource | document.RevealSource, Open: func(d *document.AssetDocument) error {
			if (d.Source == nil || !d.Source.Dirty()) && a.animatorP != nil && a.animatorP.ShowsAsset(d.Path) {
				a.ws.Show(animator.ID)
				return nil
			}
			d.Note = i18n.Tr("editor.document.controller_source", "Source view. The Animator graph is available when this controller is bound to the selected node.")
			return openCode(d)
		}},
	} {
		if err := a.ext.DocumentHandler(h); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) openDataDocument(d *document.AssetDocument) error {
	dd, err := a.data.Doc(d.Asset)
	if err != nil {
		d.Capabilities &^= document.EditSource
		d.Note = err.Error()
	} else {
		d.Binding = document.AssetBinding{Dirty: dd.Dirty, Undo: dd.Undo, Redo: dd.Redo, Save: func() error { return a.data.SaveDoc(dd) }, Discard: func() { a.data.RevertDoc(d.Asset) }}
	}
	return a.ext.DocumentView(func(doc *document.AssetDocument) func(*services.Frame, mathx.Rect) {
		p := a.inspectorPanel.NewAssetView()
		return func(f *services.Frame, r mathx.Rect) { p.DrawAsset(f, r, doc.Asset) }
	})(d)
}

func (a *App) openTableDocument(d *document.AssetDocument) error {
	t, err := a.data.Table(d.Asset)
	if err != nil {
		d.Capabilities &^= document.EditSource
		d.Note = err.Error()
		return a.ext.DocumentInspector()(d)
	}
	d.Binding = document.AssetBinding{Dirty: t.Dirty, Undo: t.Undo, Redo: t.Redo, Save: func() error { return a.data.SaveTable(t) }, Discard: func() { a.data.RevertTable(d.Asset) }}
	a.openTable(d.Asset)
	return nil
}

// readOnlyAssetView presents identity and capabilities for a known kind without a specialized viewer, never feeding binary bytes to the text editor or pretending to preview them.
func (a *App) readOnlyAssetView(d *document.AssetDocument) {
	ui := a.ctx.UI
	widgets.Label(ui, path.Base(d.Path))
	widgets.LabelDim(ui, d.Path)
	widgets.LabelDim(ui, string(d.Kind)+" · "+d.Asset.String())
	if d.Note != "" {
		widgets.LabelWrapped(ui, d.Note, true)
	}
	widgets.LabelWrapped(ui, i18n.Tr("editor.document.readonly_fallback", "Read-only asset information. This format has no specialized viewer in the enabled modules."), true)
	if d.Capabilities.Has(document.RevealSource) && widgets.Button(ui, i18n.Tr("editor.document.reveal_source", "Reveal Source")) && a.ctx.RevealPath != nil {
		a.ctx.RevealPath(d.Path)
	}
}

func (a *App) focusedAsset() *document.AssetDocument {
	if p, ok := a.ws.Panel(a.ws.Focused()); ok {
		if owner, ok := p.(interface {
			Document() *document.AssetDocument
		}); ok {
			if d := owner.Document(); d != nil {
				return d
			}
		}
	}
	if a.ws.Focused() == workspace.Inspector && a.data != nil {
		if id := a.data.Inspected(); !id.IsZero() {
			d, _ := a.ctx.DocumentForAsset(id)
			return d
		}
	}
	return nil
}

func (a *App) undoDocument() error {
	if d := a.focusedAsset(); d != nil {
		return a.ctx.UndoAsset(d)
	}
	return a.ctx.Undo()
}

func (a *App) redoDocument() error {
	if d := a.focusedAsset(); d != nil {
		return a.ctx.RedoAsset(d)
	}
	return a.ctx.Redo()
}
