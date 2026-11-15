package editor

import (
	"bytes"
	stdctx "context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/project"
	"aego/ui/core"
)

func workspaceBlankTestHost(t *testing.T, p *project.Project, pl *pipeline.Pipeline) (*context.Editor, *workspace.Workspace, *ext.Registry) {
	t.Helper()
	commands := command.New()
	docs := document.NewManager(document.Options{Root: p.Root, SourceWrite: pl.GuardSourceWrite})
	ed := &context.Editor{Project: p, Pipeline: pl, Docs: docs, Commands: commands, UI: core.New(nil)}
	ws := workspace.New(p.Root, nil)
	for _, id := range []string{workspace.Design, workspace.Art} {
		if err := ws.RegisterPreset(workspace.Preset{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	registry := ext.New(ext.Deps{Editor: ed, Workspace: ws, Commands: commands})
	if err := registry.Run("render2d", imageHooks); err != nil {
		t.Fatal(err)
	}
	ws.LoadLayout()
	return ed, ws, registry
}

func workspaceBlankFixture(t *testing.T) (*project.Project, *pipeline.Pipeline, *context.Editor, *workspace.Workspace, *ext.Registry) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Blank canvas", false)
	if err != nil {
		t.Fatal(err)
	}
	importers := pipeline.NewImporterRegistry()
	if err := importers.Register(artsource.New()); err != nil {
		t.Fatal(err)
	}
	if err := importers.Register(artsource.NewTexture()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, importers, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("fixture import failed: batch=%+v, error=%v", batch, err)
	}
	ed, ws, registry := workspaceBlankTestHost(t, p, pl)
	return p, pl, ed, ws, registry
}

func enterBlankArt(t *testing.T, ed *context.Editor, ws *workspace.Workspace, registry *ext.Registry) (*document.AssetDocument, *artedit.Document) {
	t.Helper()
	if !ws.Switch(workspace.Art) {
		t.Fatal("Art workspace is unavailable")
	}
	registry.RunTicks()
	doc := ed.Docs.AssetDocument(artWorkspaceBlankID)
	if doc == nil || !doc.Transient {
		t.Fatal("entering Art did not open an in-memory drawing")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || !model.Draft {
		t.Fatal("blank canvas did not bind the shared Art model")
	}
	return doc, model
}

// These tests describe the desired workflow; they are deferred, not evidence
// that this mixed tree has passed UI, CI, or real OS acceptance.
func TestArtWorkspaceBlankDrawSwitchAndSaveAsAsset(t *testing.T) {
	_, pl, ed, ws, registry := workspaceBlankFixture(t)
	doc, model := enterBlankArt(t, ed, ws, registry)
	if doc.Dirty() || model.PendingRecovery || model.Session.Document.Info().Canvas.Width != 32 || model.Session.Document.Info().Canvas.Height != 32 {
		t.Fatal("default blank must be a clean editable 32×32 canvas")
	}
	for _, name := range []string{"assets/drawing.aeart", "assets/drawing.aeart.meta"} {
		if got, err := pl.SourceBytes(name); err != nil || got != nil {
			t.Fatalf("entering Art created %s: %v", name, err)
		}
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 5}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if !doc.Dirty() || model.History.Cursor() != 1 {
		t.Fatal("painting did not use the document's shared dirty state and history")
	}
	if !ws.Switch(workspace.Design) {
		t.Fatal("Design workspace is unavailable")
	}
	registry.RunTicks()
	if !ws.Switch(workspace.Art) {
		t.Fatal("could not return to Art")
	}
	registry.RunTicks()
	if ed.Docs.AssetDocument(artWorkspaceBlankID) != doc || doc.Binding.Model != model || !doc.Dirty() || model.History.Cursor() != 1 {
		t.Fatal("workspace switching replaced the dirty blank drawing")
	}
	if ws.Focused() != artHandlerID+"/"+doc.Asset.String() {
		t.Fatal("returning to Art did not focus the existing drawing")
	}
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatal("first Save did not wait for a path", err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("first Save did not offer an asset path")
	}
	questions[0].Submit([]string{"assets/first-drawing.aeart"})
	id, ok := pl.Lookup("assets/first-drawing.aeart")
	if !ok {
		notice, _ := ed.LastNotice()
		t.Fatalf("confirmed drawing was not imported: %+v", notice)
	}
	saved := ed.Docs.AssetDocument(id)
	if saved == nil || saved.Transient || saved.Binding.Model != model || model.Draft || saved.Dirty() || model.History.Cursor() != 1 || ed.Docs.AssetDocument(artWorkspaceBlankID) != nil {
		t.Fatal("first Save did not transfer the same clean session and history")
	}
	pixels, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(4, 5).R != 255 || pixels.NRGBAAt(4, 5).A != 255 {
		t.Fatal("the saved drawing lost its pixels", err)
	}
	if err := model.SessionUndo(); err != nil || !saved.Dirty() {
		t.Fatal("Undo after first Save lost its history", err)
	}
	if err := model.SessionRedo(); err != nil || saved.Dirty() {
		t.Fatal("Redo did not return to the saved generation", err)
	}
}

func TestArtWorkspaceBlankSaveDoesNotOverwriteExistingFiles(t *testing.T) {
	p, _, ed, ws, registry := workspaceBlankFixture(t)
	doc, model := enterBlankArt(t, ed, ws, registry)
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 3}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatal(err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("missing Save prompt")
	}
	target := filepath.Join(p.Root, "assets", "occupied.aeart")
	other := []byte("another person's source")
	if err := os.WriteFile(target, other, 0o600); err != nil {
		t.Fatal(err)
	}
	questions[0].Submit([]string{"assets/occupied.aeart"})
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, other) {
		t.Fatal("Save overwrote an externally created destination", err)
	}
	if !doc.Dirty() || doc.Binding.Model != model || model.History.Cursor() != 1 {
		t.Fatal("a rejected Save lost unsaved pixels or history")
	}
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
		t.Fatal("the rejected destination was not reported")
	}
}

func TestArtWorkspaceBlankRecoveryRequiresExplicitAction(t *testing.T) {
	p, pl, ed, ws, registry := workspaceBlankFixture(t)
	doc, model := enterBlankArt(t, ed, ws, registry)
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 3, Y: 6}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{B: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ed.Docs.WriteAssetRecovery(doc); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(p.Root, document.RecoveryDir, "asset-"+artWorkspaceBlankID.String()+".recovery")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// A fresh manager represents reopening the project after an interruption.
	next, nextWS, nextRegistry := workspaceBlankTestHost(t, p, pl)
	reopened, recovered := enterBlankArt(t, next, nextWS, nextRegistry)
	if !recovered.PendingRecovery || reopened.Dirty() {
		t.Fatal("an earlier blank was not offered for explicit recovery")
	}
	if _, err := recovered.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 0, Y: 0}}, Brush: art.Brush{Size: 1}}); err == nil {
		t.Fatal("editing bypassed the unresolved recovery")
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, before) {
		t.Fatal("opening the blank changed the recovery", err)
	}
	if err := recovered.Recover(); err != nil || !reopened.Dirty() || recovered.PendingRecovery {
		t.Fatal("explicit blank recovery failed", err)
	}
	pixels, _, err := recovered.Capture(recovered.Session.Frame, art.Albedo)
	if err != nil || pixels.NRGBAAt(3, 6).B != 255 || pixels.NRGBAAt(3, 6).A != 255 {
		t.Fatal("explicit recovery lost the unsaved drawing", err)
	}
	if got, err := pl.SourceBytes("assets/drawing.aeart"); err != nil || got != nil {
		t.Fatal("recovery created a project asset without Save", err)
	}
}
