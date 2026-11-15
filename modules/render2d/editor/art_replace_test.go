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
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/editor/core/document"
)

// Deferred tests; no real project, asset or recovery file is used here.
func replaceableBlank(t *testing.T) (*context.Editor, *document.AssetDocument, *artedit.Document, ids.AssetID, []byte, []byte) {
	t.Helper()
	_, pl, ed, ws, registry := workspaceBlankFixture(t)
	old, err := art.New(32, 32, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	data, err := old.Encode()
	old.Close()
	if err != nil {
		t.Fatal(err)
	}
	id, err := assetops.CreateFile(ed, "assets/existing.aeart", data)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := pl.SourceBytes("assets/existing.aeart.meta")
	if err != nil || meta == nil {
		t.Fatalf("fixture metadata: %v", err)
	}
	doc, model := enterBlankArt(t, ed, ws, registry)
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 4, Y: 5}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	return ed, doc, model, id, data, meta
}

func replacementQuestion(t *testing.T, ed *context.Editor, doc *document.AssetDocument) context.Question {
	t.Helper()
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatalf("save did not open a path prompt: %v", err)
	}
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Submit == nil || q[0].DestructiveWhen == nil || q[0].DestructiveLabel == "" {
		t.Fatal("missing dynamic replacement action")
	}
	if q[0].DestructiveWhen([]string{"assets/free.aeart"}) || !q[0].DestructiveWhen([]string{"assets/existing.aeart"}) {
		t.Fatal("primary action does not follow the destination path")
	}
	return q[0]
}

func TestBlankReplaceRequiresConfirmationAndKeepsIdentity(t *testing.T) {
	ed, doc, model, id, before, metaBefore := replaceableBlank(t)
	inspected, err := ed.DocumentForAsset(id)
	if err != nil || inspected.Binding.Model != nil || inspected.Dirty() {
		t.Fatalf("could not prepare clean Inspector metadata: %v", err)
	}
	replacementQuestion(t, ed, doc).Submit([]string{"./assets/existing.aeart"})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("existing drawing did not request explicit replacement confirmation")
	}
	if got, err := ed.Pipeline.SourceBytes("assets/existing.aeart"); err != nil || !bytes.Equal(got, before) {
		t.Fatal("target changed before confirmation", err)
	}
	confirm[0].Then(1)
	if got, _ := ed.Pipeline.SourceBytes("assets/existing.aeart"); !bytes.Equal(got, before) || !doc.Dirty() {
		t.Fatal("cancelling replacement changed the source or unsaved drawing")
	}
	replacementQuestion(t, ed, doc).Submit([]string{"assets/existing.aeart"})
	confirm = ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("missing second confirmation")
	}
	confirm[0].Then(0)
	if notice, ok := ed.LastNotice(); ok && notice.Level == context.LevelError {
		t.Fatal(notice.Text)
	}
	if got, ok := ed.Pipeline.Lookup("assets/existing.aeart"); !ok || got != id {
		t.Fatal("replacement changed the existing AssetID")
	}
	if got, err := ed.Pipeline.SourceBytes("assets/existing.aeart.meta"); err != nil || !bytes.Equal(got, metaBefore) {
		t.Fatal("replacement changed metadata", err)
	}
	saved := ed.Docs.AssetDocument(id)
	if saved == nil || saved != inspected || saved.Binding.Model != model || model.Draft || saved.Dirty() || model.History.Cursor() != 1 {
		t.Fatal("replacement lost the canonical session, history or saved state")
	}
	if err := model.SessionUndo(); err != nil || !saved.Dirty() {
		t.Fatal("the drawing's Undo did not survive replacement", err)
	}
}

func TestBlankReplaceRejectsChangeDuringConfirmation(t *testing.T) {
	ed, doc, model, _, _, metaBefore := replaceableBlank(t)
	replacementQuestion(t, ed, doc).Submit([]string{"assets/existing.aeart"})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("missing confirmation")
	}
	foreign := []byte("external change while confirmation was open")
	if err := os.WriteFile(filepath.Join(ed.Project.Root, "assets", "existing.aeart"), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	confirm[0].Then(0)
	if got, err := ed.Pipeline.SourceBytes("assets/existing.aeart"); err != nil || !bytes.Equal(got, foreign) {
		t.Fatal("replacement overwrote external changes", err)
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/existing.aeart.meta"); !bytes.Equal(got, metaBefore) || !doc.Dirty() || doc.Binding.Model != model {
		t.Fatal("rejected replacement changed metadata or lost unsaved pixels")
	}
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
		t.Fatal("the changed destination was not reported")
	}
}

func TestBlankReplaceDoesNotDiscardTargetRecovery(t *testing.T) {
	ed, doc, model, id, before, metaBefore := replaceableBlank(t)
	dir := filepath.Join(ed.Project.Root, document.RecoveryDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	recoveryPath := filepath.Join(dir, "asset-"+id.String()+".recovery")
	recovery := []byte("earlier unsaved drawing")
	if err := os.WriteFile(recoveryPath, recovery, 0600); err != nil {
		t.Fatal(err)
	}
	replacementQuestion(t, ed, doc).Submit([]string{"assets/existing.aeart"})
	if q := ed.TakeQuestions(); len(q) != 0 {
		t.Fatal("replacement offered despite pending destination recovery")
	}
	if got, err := os.ReadFile(recoveryPath); err != nil || !bytes.Equal(got, recovery) {
		t.Fatal("destination recovery was changed", err)
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/existing.aeart"); !bytes.Equal(got, before) || !doc.Dirty() || doc.Binding.Model != model {
		t.Fatal("rejected replacement changed the source or draft")
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/existing.aeart.meta"); !bytes.Equal(got, metaBefore) {
		t.Fatal("rejected replacement changed metadata")
	}
}

func TestReplacementRejectsLostFrameOrSurfaceReferences(t *testing.T) {
	old := art.Info{Frames: []art.Frame{{ID: "a"}, {ID: "b"}}, Tags: []art.Tag{{ID: "tag"}}, Cels: []art.Cel{{Channel: art.Normal}}}
	if err := artReplacementKeepsReferences(old, old); err != nil {
		t.Fatal(err)
	}
	for _, next := range []art.Info{
		{Frames: old.Frames[:1], Tags: old.Tags, Cels: old.Cels},
		{Frames: old.Frames, Cels: old.Cels},
		{Frames: old.Frames, Tags: old.Tags},
	} {
		if err := artReplacementKeepsReferences(old, next); err == nil {
			t.Fatal("replacement silently dropped a frame, tag or surface identity")
		}
	}
}

func TestPNGFirstSaveCanReplaceCompatibleDrawing(t *testing.T) {
	ed, source, frames, originalPNG := linkedArtEditor(t)
	doc, err := ed.OpenAsset(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	model := doc.Binding.Model.(*artedit.Document)
	data, err := model.Session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	id, err := assetops.CreateFile(ed, "assets/existing.aeart", data)
	if err != nil {
		t.Fatal(err)
	}
	oldMeta, err := ed.Pipeline.SourceBytes("assets/existing.aeart.meta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{G: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(doc); !errors.Is(err, document.ErrSavePending) {
		t.Fatal(err)
	}
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].DestructiveWhen == nil || !q[0].DestructiveWhen([]string{"assets/existing.aeart"}) {
		t.Fatal("PNG save did not identify the existing source")
	}
	q[0].Submit([]string{"assets/existing.aeart"})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("PNG replacement did not request confirmation")
	}
	confirm[0].Then(0)
	if notice, ok := ed.LastNotice(); ok && notice.Level == context.LevelError {
		t.Fatal(notice.Text)
	}
	meta, ok := ed.Pipeline.MetaOf(source)
	if !ok {
		t.Fatal("PNG metadata disappeared")
	}
	link, err := artsource.TextureLinkOf(&meta)
	if err != nil || link == nil || link.Source != id || len(link.Frames) != 2 || link.Frames[1].Asset != frames[1] {
		t.Fatalf("replacement lost PNG frame references: %v", err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/linked.png"); err != nil || !bytes.Equal(got, originalPNG) {
		t.Fatal("PNG replacement changed the original PNG", err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/existing.aeart.meta"); err != nil || !bytes.Equal(got, oldMeta) {
		t.Fatal("PNG replacement changed the existing source metadata", err)
	}
	if saved := ed.Docs.AssetDocument(id); saved == nil || saved.Binding.Model != model || model.Draft || saved.Dirty() {
		t.Fatal("PNG replacement did not retain the saved drawing session")
	}
}

func TestArtSavePathAndReplaceHintUseTheSameRelativePath(t *testing.T) {
	ed, _, _, _, _, _ := replaceableBlank(t)
	for _, input := range []string{"assets/existing.aeart", "./assets/existing.aeart", " ././assets/existing.aeart "} {
		name, err := artSavePath(input)
		if err != nil || name != "assets/existing.aeart" || !artSaveReplaces(ed, []string{input}) {
			t.Fatalf("relative destination %q: name=%q replace=%v error=%v", input, name, artSaveReplaces(ed, []string{input}), err)
		}
	}
	for _, input := range []string{"../assets/existing.aeart", "assets/../existing.aeart", "/assets/existing.aeart", "assets\\existing.aeart", "assets/a:b.aeart"} {
		if _, err := artSavePath(input); err == nil || artSaveReplaces(ed, []string{input}) {
			t.Fatalf("unsafe destination accepted: %q", input)
		}
	}
}

func TestArtSaveAsExistingTargetRequiresConfirmationAndRetainsOwner(t *testing.T) {
	p := artActionFixture(t)
	ed, doc, model := p.ctx, p.doc, p.model
	data, err := model.Session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	id, err := assetops.CreateFile(ed, "assets/save-as-target.aeart", data)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ed.Pipeline.SourceBytes("assets/save-as-target.aeart.meta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 2, Y: 3}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{B: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	cursor := model.History.Cursor()
	p.askSaveAs()
	questions := ed.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil || questions[0].DestructiveWhen == nil || questions[0].DestructiveLabel == "" {
		t.Fatal("Save As did not create a dynamic replacement prompt")
	}
	if questions[0].DestructiveWhen([]string{"assets/new-copy.aeart"}) || !questions[0].DestructiveWhen([]string{"./assets/save-as-target.aeart"}) {
		t.Fatal("Save As did not identify the existing relative destination")
	}
	questions[0].Submit([]string{"./assets/save-as-target.aeart"})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("Save As did not request explicit replacement confirmation")
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/save-as-target.aeart"); !bytes.Equal(got, data) {
		t.Fatal("target changed before confirmation")
	}
	confirm[0].Then(0)
	if p.err != nil {
		t.Fatal(p.err)
	}
	if got, _ := ed.Pipeline.SourceBytes("assets/save-as-target.aeart.meta"); !bytes.Equal(got, meta) {
		t.Fatal("Save As changed the destination identity or metadata")
	}
	if got, ok := ed.Pipeline.Lookup("assets/save-as-target.aeart"); !ok || got != id {
		t.Fatal("Save As changed the destination AssetID")
	}
	if doc.Binding.Model != model || !doc.Dirty() || model.History.Cursor() != cursor {
		t.Fatal("Save As discarded or marked the original drawing clean")
	}
	copyDoc := ed.Docs.AssetDocument(id)
	if copyDoc == nil || copyDoc == doc || copyDoc.Binding.Model == model {
		t.Fatal("confirmed destination did not open as an independent document")
	}
}

func TestArtSaveAsRejectsDestinationAppearingOrChanging(t *testing.T) {
	for _, mutate := range []string{"appears", "source", "metadata"} {
		t.Run(mutate, func(t *testing.T) {
			p := artActionFixture(t)
			ed, doc, model := p.ctx, p.doc, p.model
			name := "assets/save-as-race.aeart"
			if mutate != "appears" {
				data, err := model.Session.Document.Encode()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := assetops.CreateFile(ed, name, data); err != nil {
					t.Fatal(err)
				}
			}
			p.askSaveAs()
			q := ed.TakeQuestions()
			if len(q) != 1 || q[0].DestructiveWhen == nil {
				t.Fatal("missing Save As prompt")
			}
			foreign := []byte("external bytes; preserve exactly")
			changed := name
			if mutate == "metadata" {
				changed += ".meta"
			}
			if mutate == "appears" {
				if q[0].DestructiveWhen([]string{name}) {
					t.Fatal("absent destination appeared as a replacement")
				}
				if err := os.WriteFile(filepath.Join(ed.Project.Root, filepath.FromSlash(changed)), foreign, 0600); err != nil {
					t.Fatal(err)
				}
				if !q[0].DestructiveWhen([]string{"./" + name}) {
					t.Fatal("unindexed destination appearing during the prompt was not marked Replace")
				}
				q[0].Submit([]string{name})
			} else {
				q[0].Submit([]string{name})
				confirm := ed.TakeQuestions()
				if len(confirm) != 1 || confirm[0].Then == nil {
					t.Fatal("missing replacement confirmation")
				}
				if err := os.WriteFile(filepath.Join(ed.Project.Root, filepath.FromSlash(changed)), foreign, 0600); err != nil {
					t.Fatal(err)
				}
				confirm[0].Then(0)
			}
			if got, err := os.ReadFile(filepath.Join(ed.Project.Root, filepath.FromSlash(changed))); err != nil || !bytes.Equal(got, foreign) {
				t.Fatal("Save As overwrote a concurrent destination change", err)
			}
			if p.err == nil || doc.Binding.Model != model || !doc.Dirty() {
				t.Fatal("rejected Save As lost the error or dirty drawing")
			}
		})
	}
}

func TestArtSaveAsProtectsOpenDestinationAndRecovery(t *testing.T) {
	for _, state := range []string{"open-clean", "open-dirty", "recovery", "metadata-dirty"} {
		t.Run(state, func(t *testing.T) {
			p := artActionFixture(t)
			ed, doc, model := p.ctx, p.doc, p.model
			data, err := model.Session.Document.Encode()
			if err != nil {
				t.Fatal(err)
			}
			id, err := assetops.CreateFile(ed, "assets/protected.aeart", data)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := ed.Pipeline.SourceBytes("assets/protected.aeart.meta")
			if err != nil {
				t.Fatal(err)
			}
			switch state {
			case "open-clean", "open-dirty":
				other, err := ed.OpenAsset(id)
				if err != nil {
					t.Fatal(err)
				}
				if state == "open-dirty" {
					bound := other.Binding.Model.(*artedit.Document)
					if _, err := bound.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 3, Y: 3}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}); err != nil {
						t.Fatal(err)
					}
				}
			case "metadata-dirty":
				other, err := ed.DocumentForAsset(id)
				if err != nil || other.Metadata == nil {
					t.Fatal("cannot open target metadata", err)
				}
				other.Metadata.Code.ReplaceSnapshot([]byte("unsaved metadata"), false)
			case "recovery":
				dir := filepath.Join(ed.Project.Root, document.RecoveryDir)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "asset-"+id.String()+".recovery"), []byte("pending recovery"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			p.askSaveAs()
			q := ed.TakeQuestions()
			if len(q) != 1 || q[0].DestructiveWhen == nil || !q[0].DestructiveWhen([]string{"assets/protected.aeart"}) {
				t.Fatal("existing target was not marked Replace")
			}
			q[0].Submit([]string{"assets/protected.aeart"})
			if len(ed.TakeQuestions()) != 0 || p.err == nil {
				t.Fatal("protected target was offered for replacement")
			}
			if got, _ := ed.Pipeline.SourceBytes("assets/protected.aeart"); !bytes.Equal(got, data) {
				t.Fatal("protected destination source changed")
			}
			if got, _ := ed.Pipeline.SourceBytes("assets/protected.aeart.meta"); !bytes.Equal(got, meta) {
				t.Fatal("protected destination metadata changed")
			}
			if doc.Binding.Model != model || !doc.Dirty() {
				t.Fatal("the original dirty drawing was lost")
			}
		})
	}
}

func TestArtSaveAsCurrentSourceRejectsChangedMetadata(t *testing.T) {
	p := artActionFixture(t)
	ed, doc := p.ctx, p.doc
	before, err := ed.Pipeline.SourceBytes(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	p.askSaveAs()
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].DestructiveWhen == nil || !q[0].DestructiveWhen([]string{doc.Path}) {
		t.Fatal("current source was not marked Replace")
	}
	q[0].Submit([]string{doc.Path})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("current source did not require explicit confirmation")
	}
	foreign := []byte("changed metadata from another process")
	if err := os.WriteFile(filepath.Join(ed.Project.Root, filepath.FromSlash(doc.Path+".meta")), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	confirm[0].Then(0)
	if p.err == nil || !doc.Dirty() {
		t.Fatal("changed metadata did not block saving the current drawing")
	}
	if got, _ := ed.Pipeline.SourceBytes(doc.Path); !bytes.Equal(got, before) {
		t.Fatal("current source changed after metadata conflict")
	}
	if got, _ := ed.Pipeline.SourceBytes(doc.Path + ".meta"); !bytes.Equal(got, foreign) {
		t.Fatal("external metadata was overwritten")
	}
}

func TestArtSaveAsCurrentSourceRejectsEarlierExternalEdit(t *testing.T) {
	p := artActionFixture(t)
	ed, doc, model := p.ctx, p.doc, p.model
	p.askSaveAs()
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Submit == nil {
		t.Fatal("missing Save As prompt")
	}
	foreign := []byte("external source edit made before choosing the path")
	if err := os.WriteFile(filepath.Join(ed.Project.Root, filepath.FromSlash(doc.Path)), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	q[0].Submit([]string{doc.Path})
	confirm := ed.TakeQuestions()
	if len(confirm) != 1 || confirm[0].Then == nil {
		t.Fatal("missing current-source replacement confirmation")
	}
	confirm[0].Then(0)
	if p.err == nil || doc.Binding.Model != model || !doc.Dirty() {
		t.Fatal("the BinarySource generation guard did not preserve the dirty drawing")
	}
	if got, err := ed.Pipeline.SourceBytes(doc.Path); err != nil || !bytes.Equal(got, foreign) {
		t.Fatal("Save As overwrote the earlier external source edit", err)
	}
}

func TestArtSaveAsExistingUnindexedSourceIsNotOverwritten(t *testing.T) {
	p := artActionFixture(t)
	ed, doc, model := p.ctx, p.doc, p.model
	name := "assets/unindexed.aeart"
	data, err := model.Session.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ed.Pipeline.NewSourceMeta(name)
	if err != nil {
		t.Fatal(err)
	}
	metaBytes := pipeline.WriteMeta(meta)
	for file, contents := range map[string][]byte{name: data, name + ".meta": metaBytes} {
		if err := os.WriteFile(filepath.Join(ed.Project.Root, filepath.FromSlash(file)), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := ed.Pipeline.Lookup(name); ok {
		t.Fatal("fixture unexpectedly indexed the destination")
	}
	p.askSaveAs()
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].DestructiveWhen == nil || !q[0].DestructiveWhen([]string{"./" + name}) {
		t.Fatal("unindexed existing file was not shown as Replace")
	}
	q[0].Submit([]string{"./" + name})
	if p.err == nil || len(ed.TakeQuestions()) != 0 {
		t.Fatal("an unindexed identity was accepted for replacement")
	}
	for file, want := range map[string][]byte{name: data, name + ".meta": metaBytes} {
		if got, err := ed.Pipeline.SourceBytes(file); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("unindexed %s changed: %v", file, err)
		}
	}
	if doc.Binding.Model != model || !doc.Dirty() {
		t.Fatal("rejected unindexed replacement lost the source drawing")
	}
}
