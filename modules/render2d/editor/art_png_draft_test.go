package editor

import (
	"bytes"
	stdctx "context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/ui/core"
)

// These scenarios are deliberately retained for the deferred test pass; they
// are not evidence that the current mixed checkout or native UI has passed.
func TestPNGOpenIsInMemoryAndFirstSaveKeepsHistoryAndIDs(t *testing.T) {
	ed, source, frames, original := linkedArtEditor(t)
	metaBefore, err := ed.Pipeline.SourceBytes("assets/linked.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ed.OpenAsset(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok || !model.Draft || doc.Asset != source || len(model.Session.Document.Info().Frames) != 2 || len(ed.TakeQuestions()) != 0 {
		t.Fatal("PNG did not open directly as one in-memory sliced drawing")
	}
	before, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || before.NRGBAAt(0, 0).R != 100 {
		t.Fatalf("selected sliced frame lost its original pixels: %v", err)
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/linked.png.meta"); !bytes.Equal(got, metaBefore) {
		t.Fatal("opening PNG changed its metadata")
	}
	if _, ok := ed.Pipeline.Lookup("assets/linked.aeart"); ok {
		t.Fatal("opening PNG created an editable source")
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{1, 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, B: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatalf("first Save did not wait for an explicit destination: %v", err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("first Save did not present its source path and metadata warning")
	}
	questions[0].Submit([]string{"assets/linked.aeart"})
	if notice, ok := ed.LastNotice(); ok && notice.Level == context.LevelError {
		t.Fatal(notice.Text)
	}
	id, ok := ed.Pipeline.Lookup("assets/linked.aeart")
	if !ok {
		t.Fatal("confirmed source was not indexed")
	}
	saved := ed.Docs.AssetDocument(id)
	if saved == nil || saved.Binding.Model != model || model.Draft || saved.Dirty() || model.History.Cursor() != 1 {
		t.Fatal("first Save did not transfer the same clean session and history")
	}
	meta, _ := ed.Pipeline.MetaOf(source)
	link, err := artsource.TextureLinkOf(&meta)
	if err != nil || link == nil || link.Source != id || len(link.Frames) != 2 || link.Frames[1].Asset != frames[1] {
		t.Fatalf("first Save lost original frame identities: %v", err)
	}
	if png, _ := ed.Pipeline.SourceBytes("assets/linked.png"); !bytes.Equal(png, original) {
		t.Fatal("first Save overwrote the original PNG")
	}
	if err := model.SessionUndo(); err != nil || !saved.Dirty() {
		t.Fatal("Undo after first Save lost the pre-save history", err)
	}
	if err := model.SessionRedo(); err != nil || saved.Dirty() {
		t.Fatal("Redo did not restore the saved generation", err)
	}
	if _, err := ed.OpenAsset(frames[0]); err != nil || model.Session.Frame != link.Frames[0].Frame || saved.Binding.Model != model {
		t.Fatal("reopening the PNG did not focus its canonical linked document", err)
	}
}

// Exercises the reported sequence without touching a real project: first save
// of an imported PNG, a later normal save, then Save As to the same source.
func TestPNGFirstSaveThenSaveAsCurrentSourceRequiresReplace(t *testing.T) {
	ed, _, frames, original := linkedArtEditor(t)
	doc, err := ed.OpenAsset(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	paint := func(x int) {
		t.Helper()
		_, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: x % 2, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: uint8(160 + x*30), A: 255}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	paint(0)
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatalf("first Save did not request a destination: %v", err)
	}
	first := ed.TakeQuestions()
	if len(first) != 1 || first[0].Submit == nil {
		t.Fatal("missing first-save prompt")
	}
	first[0].Submit([]string{"./assets/linked.aeart"})
	id, ok := ed.Pipeline.Lookup("assets/linked.aeart")
	if !ok {
		t.Fatal("first Save did not index the linked source")
	}
	saved := ed.Docs.AssetDocument(id)
	if saved == nil || saved.Binding.Model != model || model.Draft {
		t.Fatal("first Save did not retain the canonical model")
	}
	paint(1)
	if err := ed.SaveAsset(saved); err != nil {
		t.Fatalf("normal subsequent Save failed: %v", err)
	}
	paint(2)
	cursor := model.History.Cursor()
	panel := &artPanel{ctx: ed, doc: saved, model: model, view: &artView{}}
	panel.askSaveAs()
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].DestructiveWhen == nil || !q[0].DestructiveWhen([]string{"./assets/linked.aeart"}) {
		t.Fatal("Save As did not show Replace for the current source")
	}
	before, err := ed.Pipeline.SourceBytes("assets/linked.aeart")
	if err != nil {
		t.Fatal(err)
	}
	q[0].Submit([]string{"./assets/linked.aeart"})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("current source replacement did not request confirmation")
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/linked.aeart"); !bytes.Equal(got, before) || !saved.Dirty() {
		t.Fatal("the current source was saved before confirmation")
	}
	confirm[0].Then(0)
	if panel.err != nil || saved.Dirty() || saved.Binding.Model != model || model.History.Cursor() != cursor {
		t.Fatal("confirmed Save As lost its source generation or history", panel.err)
	}
	if png, _ := ed.Pipeline.SourceBytes("assets/linked.png"); !bytes.Equal(png, original) {
		t.Fatal("subsequent saves changed the original PNG")
	}
}

func TestPNGFirstSaveRejectsExternalChangeAndKeepsDirtyPixels(t *testing.T) {
	for _, changed := range []string{"assets/linked.png", "assets/linked.png.meta", "assets/linked.aeart"} {
		t.Run(changed, func(t *testing.T) {
			ed, _, frames, _ := linkedArtEditor(t)
			doc, err := ed.OpenAsset(frames[1])
			if err != nil {
				t.Fatal(err)
			}
			model := doc.Binding.Model.(*artedit.Document)
			if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{1, 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
				t.Fatal(err)
			}
			if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
				t.Fatal(err)
			}
			prompt := ed.TakeQuestions()[0]
			foreign := []byte("external owner bytes")
			if err := os.WriteFile(filepath.Join(ed.Project.Root, filepath.FromSlash(changed)), foreign, 0600); err != nil {
				t.Fatal(err)
			}
			prompt.Submit([]string{"assets/linked.aeart"})
			if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
				t.Fatal("concurrent change was not reported")
			}
			if got, err := os.ReadFile(filepath.Join(ed.Project.Root, filepath.FromSlash(changed))); err != nil || !bytes.Equal(got, foreign) {
				t.Fatal("first Save overwrote the external change", err)
			}
			if doc.Binding.Model != model || !doc.Dirty() || model.History.Cursor() != 1 {
				t.Fatal("failed Save lost the dirty session or history")
			}
		})
	}
}

func TestPNGDraftNeverSubstitutesAnotherFrameForUnknownAsset(t *testing.T) {
	ed, source, frames, _ := linkedArtEditor(t)
	doc, err := ed.OpenAsset(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	info := model.Session.Document.Info()
	draft := &pngArtDraft{doc: doc, model: model, frames: []artsource.TextureLinkFrame{
		{Asset: frames[0], Frame: info.Frames[0].ID},
		{Asset: frames[1], Frame: info.Frames[1].ID},
	}}
	selected := model.Session.Frame
	if err := draft.selectFrame(ids.NewAssetID()); err == nil || model.Session.Frame != selected {
		t.Fatal("unknown frame silently switched to a different sprite", err)
	}
	if err := draft.selectFrame(source); err != nil || model.Session.Frame != info.Frames[0].ID {
		t.Fatal("opening the sheet itself did not select its first frame", err)
	}
	if err := draft.selectFrame(frames[1]); err != nil || model.Session.Frame != selected {
		t.Fatal("the original frame identity no longer selects its own pixels", err)
	}
}

// A new manager represents the next editor process after an interrupted run.
// It shares the fixture pipeline only so the test does not alter real projects.
func nextPNGEditor(t *testing.T, old *context.Editor) *context.Editor {
	t.Helper()
	commands := command.New()
	ed := &context.Editor{Project: old.Project, Pipeline: old.Pipeline, Docs: document.NewManager(document.Options{Root: old.Project.Root}), Commands: commands, UI: core.New(nil)}
	registry := ext.New(ext.Deps{Editor: ed, Workspace: workspace.New(old.Project.Root, nil), Commands: commands})
	if err := registry.Run("render2d", imageHooks); err != nil {
		t.Fatal(err)
	}
	return ed
}

func makePendingSourceJournal(t *testing.T, root string) (string, string) {
	t.Helper()
	journal := filepath.Join(root, ".aego", "source-transactions", "active")
	if err := os.MkdirAll(journal, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(journal, "unresolved")
	if err := os.WriteFile(marker, []byte("preserve this journal"), 0o600); err != nil {
		t.Fatal(err)
	}
	return journal, marker
}

func TestPNGOpenRejectsPendingSourceJournal(t *testing.T) {
	ed, _, frames, original := linkedArtEditor(t)
	meta, err := ed.Pipeline.SourceBytes("assets/linked.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	_, marker := makePendingSourceJournal(t, ed.Project.Root)
	doc, err := ed.OpenAsset(frames[0])
	if err == nil || !strings.Contains(err.Error(), "source transaction") {
		t.Fatalf("pending journal did not block the in-memory import: %v", err)
	}
	if doc != nil && doc.Binding.Model != nil {
		t.Fatal("a model was bound from potentially incomplete source inputs")
	}
	for name, want := range map[string][]byte{"assets/linked.png": original, "assets/linked.png.meta": meta} {
		if got, err := ed.Pipeline.SourceBytes(name); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("opening changed %s: %v", name, err)
		}
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "preserve this journal" {
		t.Fatalf("opening altered the source journal: %q, %v", got, err)
	}
}

func TestPNGRecoveryAndFirstSaveRespectPendingSourceJournal(t *testing.T) {
	ed, source, frames, original := linkedArtEditor(t)
	doc, err := ed.OpenAsset(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	frameID := model.Session.Frame
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{1, 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ed.Docs.WriteAssetRecovery(doc); err != nil {
		t.Fatal(err)
	}
	recoveryPath := filepath.Join(ed.Project.Root, document.RecoveryDir, "asset-"+source.String()+".recovery")
	recovery, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatal("first save did not request confirmation", err)
	}
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("missing first-save prompt")
	}
	next := nextPNGEditor(t, ed)
	reopened, err := next.OpenAsset(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	recovered := reopened.Binding.Model.(*artedit.Document)
	if !recovered.PendingRecovery {
		t.Fatal("the existing recovery was not offered explicitly")
	}
	journal, marker := makePendingSourceJournal(t, ed.Project.Root)
	if err := recovered.Recover(); err == nil || !strings.Contains(err.Error(), "source transaction") {
		t.Fatalf("recovery was allowed while the source journal was pending: %v", err)
	}
	questions[0].Submit([]string{"assets/linked.aeart"})
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
		t.Fatal("a delayed save did not report the pending journal")
	}
	if !doc.Dirty() || doc.Binding.Model != model || model.History.Cursor() != 1 || !recovered.PendingRecovery {
		t.Fatal("the rejected actions lost unsaved pixels, history, or the recovery decision")
	}
	if got, err := os.ReadFile(recoveryPath); err != nil || !bytes.Equal(got, recovery) {
		t.Fatal("the rejected actions changed the recovery file", err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/linked.png"); err != nil || !bytes.Equal(got, original) {
		t.Fatal("the rejected actions changed the original PNG", err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/linked.aeart"); err != nil || got != nil {
		t.Fatal("the rejected save created an art source", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "preserve this journal" {
		t.Fatal("the rejected actions changed the journal", err)
	}
	// This test-created fixture is resolved explicitly; real recovery is never
	// removed or applied automatically by the editor.
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Recover(); err != nil || !reopened.Dirty() || recovered.PendingRecovery {
		t.Fatalf("explicit recovery failed after resolving the journal: %v", err)
	}
	if recovered.Session.Frame != frameID {
		t.Fatal("recovery lost the selected sliced frame")
	}
	pixels, _, err := recovered.Capture(frameID, art.Albedo)
	if err != nil || pixels.NRGBAAt(1, 1).G != 255 {
		t.Fatalf("recovery did not retain the edited pixels: %v", err)
	}
}
