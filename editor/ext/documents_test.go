package ext

import (
	"bytes"
	"os"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/platform/input"
	"aego/ui/core"
)

func TestAssetInspectorOpensSharedDocumentTabAndClosesItsJournal(t *testing.T) {
	r, deps := setup(t)
	ed := deps.Editor
	ed.Docs = document.NewManager(document.Options{Root: t.TempDir()})
	ed.UI = core.New(nil)
	selected, rendered := 0, 0
	ed.InspectAsset = func(ids.AssetID) { selected++ }
	if err := r.Run("material", func(r *Registry) error {
		if err := r.AssetInspector(string(asset.ArtMaterial), func(_ *context.Editor, id ids.AssetID) {
			if id.Lo != 42 {
				t.Fatal("view lost its explicit document identity")
			}
			rendered++
		}); err != nil {
			return err
		}
		return r.DocumentHandler(document.Handler{ID: "material.document", Kinds: []asset.ArtifactKind{asset.ArtMaterial}, TextSource: true, Capabilities: document.View | document.EditSource | document.Preview, Open: r.DocumentInspector()})
	}); err != nil {
		t.Fatal(err)
	}
	const original = "material version=1 {}\n"
	if err := document.WriteAtomic(ed.Docs.Abs("assets/test.aemat"), []byte(original)); err != nil {
		t.Fatal(err)
	}
	req := document.AssetRequest{Asset: ids.AssetID{Lo: 42}, Path: "assets/test.aemat", Kind: asset.ArtMaterial}
	d, err := ed.Docs.OpenAsset(req)
	if err != nil {
		t.Fatal(err)
	}
	id := "asset.document/" + d.Asset.String()
	panel, ok := deps.Workspace.Panel(id)
	if !ok || !deps.Workspace.Visible(id) || deps.Workspace.Focused() != id || selected != 0 {
		t.Fatal("document opening only selected Inspector or failed to focus a tab")
	}
	d.Source.Code.ReplaceSnapshot([]byte("material version=1 { extra=1 }\n"), false)
	again, err := ed.Docs.OpenAsset(req)
	other, _ := deps.Workspace.Panel(id)
	if err != nil || again != d || other != panel || len(ed.Docs.AssetDocuments()) != 1 || !panel.(workspace.Dirtier).Dirty() {
		t.Fatal("reopening duplicated the view/model or lost its dirty journal")
	}
	ed.UI.Begin(&input.State{}, mathx.Vec2{X: 640, Y: 480}, 1)
	panel.Draw(&services.Frame{UI: ed.UI}, mathx.Rect{W: 640, H: 480})
	ed.UI.End()
	if rendered != 1 || selected != 0 {
		t.Fatal("document view did not reuse the registered asset editor")
	}
	panel.(workspace.Closer).RequestClose()
	questions := ed.TakeQuestions()
	if len(questions) != 1 {
		t.Fatal("dirty document closed without a Save/Discard/Cancel choice")
	}
	questions[0].Then(2)
	if !deps.Workspace.Has(id) || ed.Docs.AssetDocument(d.Asset) != d {
		t.Fatal("Cancel discarded the document")
	}
	panel.(workspace.Closer).RequestClose()
	ed.TakeQuestions()[0].Then(1)
	if deps.Workspace.Has(id) || ed.Docs.AssetDocument(d.Asset) != nil || ed.Docs.Script(req.Path) != nil {
		t.Fatal("Discard retained a tab or private editable copy")
	}
	if disk, err := os.ReadFile(ed.Docs.Abs(req.Path)); err != nil || !bytes.Equal(disk, []byte(original)) {
		t.Fatal("Discard wrote the source")
	}
}

func TestModuleDocumentHandlerHasOwnedLifecycle(t *testing.T) {
	r, deps := setup(t)
	deps.Editor.Docs = document.NewManager(document.Options{Root: t.TempDir()})
	const kind asset.ArtifactKind = "quest-package"
	opened := 0
	if err := r.Run("quest", func(r *Registry) error {
		return r.DocumentHandler(document.Handler{ID: "quest.viewer", Kinds: []asset.ArtifactKind{kind}, Roles: []string{"story"}, Capabilities: document.View | document.RevealSource,
			Open: func(d *document.AssetDocument) error {
				opened++
				if d.Role != "story" {
					t.Fatal("semantic role lost")
				}
				return nil
			}})
	}); err != nil {
		t.Fatal(err)
	}
	req := document.AssetRequest{Asset: ids.AssetID{Lo: 7}, Path: "assets/quest.pack", Kind: kind, Role: "story"}
	d, err := deps.Editor.Docs.OpenAsset(req)
	if err != nil || opened != 1 || d.Handler != "quest.viewer" || d.Source != nil {
		t.Fatal("new module did not open through generic route", err)
	}
	r.Disable("quest", "test shutdown")
	r.Flush()
	again, err := deps.Editor.Docs.OpenAsset(req)
	if err != nil || again != d || again.Handler != "asset.readonly" || opened != 1 {
		t.Fatal("disabled module retained opening handler or lost asset identity", err)
	}
}

type ownedDocumentPanel struct {
	doc   *document.AssetDocument
	close func()
}

func (p *ownedDocumentPanel) ID() string                        { return "test.document/" + p.doc.Asset.String() }
func (p *ownedDocumentPanel) Title() string                     { return p.doc.Caption() }
func (p *ownedDocumentPanel) Draw(*services.Frame, mathx.Rect)  {}
func (p *ownedDocumentPanel) Document() *document.AssetDocument { return p.doc }
func (p *ownedDocumentPanel) RequestClose()                     { p.close() }

func TestExtensionDocumentTabsForwardIdentityCloseAndOwnedLifetime(t *testing.T) {
	r, deps := setup(t)
	deps.Editor.Docs = document.NewManager(document.Options{Root: t.TempDir()})
	const kind asset.ArtifactKind = "binary-package"
	closed := 0
	if err := r.Run("test-docs", func(r *Registry) error {
		open := r.DocumentPanelFactory(func(d *document.AssetDocument) workspace.Panel {
			p := &ownedDocumentPanel{doc: d}
			p.close = func() { closed++; r.ClosePanel(p.ID()) }
			return p
		})
		return r.DocumentHandler(document.Handler{ID: "test.binary", Kinds: []asset.ArtifactKind{kind}, Capabilities: document.View, Open: open})
	}); err != nil {
		t.Fatal(err)
	}
	req := document.AssetRequest{Asset: ids.AssetID{Lo: 33}, Kind: kind, Path: "assets/blob.bin"}
	d, err := deps.Editor.Docs.OpenAsset(req)
	if err != nil {
		t.Fatal(err)
	}
	id := "test.document/" + d.Asset.String()
	p, ok := deps.Workspace.Panel(id)
	if !ok || !deps.Workspace.Visible(id) || p.(interface {
		Document() *document.AssetDocument
	}).Document() != d {
		t.Fatal("extension document did not use the normal workspace identity")
	}
	if _, err := deps.Editor.Docs.OpenAsset(req); err != nil || len(r.own("test-docs").panels) != 1 {
		t.Fatal("opening the same document registered another workspace panel", err)
	}
	p.(workspace.Closer).RequestClose()
	if closed != 1 || deps.Workspace.Has(id) || len(r.own("test-docs").panels) != 0 {
		t.Fatal("safe extension wrapper swallowed close or retained a historical panel")
	}
	if _, err := deps.Editor.Docs.OpenAsset(req); err != nil {
		t.Fatal(err)
	}
	r.Disable("test-docs", "test teardown")
	r.Flush()
	if deps.Workspace.Has(id) || deps.Editor.Docs.Handlers().Resolve(kind, req.Path, "").ID != "asset.readonly" {
		t.Fatal("module disable left a document tab or handler alive")
	}
}
