package editor

import (
	"bytes"
	stdctx "context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/artedit"
	"aego/editor/core/assetops"
	"aego/editor/core/context"
	"aego/engine/i18n"
)

func linkedArtEditor(t *testing.T) (*context.Editor, ids.AssetID, [2]ids.AssetID, []byte) {
	t.Helper()
	ed, _, _ := importTestEditor(t)
	pixels := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x*40), G: 100, B: 200, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, pixels); err != nil {
		t.Fatal(err)
	}
	id, err := assetops.CreateFile(ed, "assets/linked.png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	before, err := ed.Pipeline.SourceBytes("assets/linked.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := pipeline.ParseMeta(before, "assets/linked.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	frames := [2]ids.AssetID{ids.NewAssetID(), ids.NewAssetID()}
	meta.Subs = []pipeline.SubAsset{
		{ID: frames[0], Name: "idle", Rect: [4]int{0, 0, 2, 2}},
		{ID: frames[1], Name: "run", Rect: [4]int{2, 0, 2, 2}},
	}
	batch, err := ed.Pipeline.ApplySources([]pipeline.SourceEdit{{Path: "assets/linked.png.meta", Before: before, After: pipeline.WriteMeta(meta)}}, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("cannot prepare fixture: batch=%+v error=%v", batch, err)
	}
	ed.ApplyBatch(batch)
	return ed, id, frames, bytes.Clone(data.Bytes())
}

func linkedSourcePrompt(t *testing.T, ed *context.Editor) context.Question {
	t.Helper()
	qs := ed.TakeQuestions()
	if len(qs) != 1 || qs[0].Then == nil || !strings.Contains(qs[0].Message, "metadata") || !strings.Contains(qs[0].Message, "shared asset") {
		t.Fatalf("missing visible confirmation of metadata and shared asset changes: %+v", qs)
	}
	qs[0].Then(0)
	qs = ed.TakeQuestions()
	if len(qs) != 1 || qs[0].Submit == nil {
		t.Fatalf("missing source path prompt: %+v", qs)
	}
	return qs[0]
}

// Keep exercising the earlier explicit linker independently of the default
// PNG-open route, which now creates only an in-memory draft until Save.
func askLegacyLinkedSource(t *testing.T, ed *context.Editor, id ids.AssetID) {
	t.Helper()
	doc, err := ed.DocumentForAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := pipeline.ParseMeta(doc.Metadata.Code.Bytes(), doc.Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := askLinkedArtSource(ed, doc, meta); err != nil {
		t.Fatal(err)
	}
}

func TestLinkedSourceConfirmationAndDirtyDocument(t *testing.T) {
	ed, source, frames, original := linkedArtEditor(t)
	askLegacyLinkedSource(t, ed, frames[1])
	qs := ed.TakeQuestions()
	if len(qs) != 1 || qs[0].Then == nil {
		t.Fatalf("missing confirmation: %+v", qs)
	}
	qs[0].Then(1)
	if _, ok := ed.Pipeline.Lookup("assets/linked.aeart"); ok || len(ed.TakeQuestions()) != 0 {
		t.Fatal("cancel created a source or opened a path prompt")
	}
	askLegacyLinkedSource(t, ed, frames[1])
	linkedSourcePrompt(t, ed).Submit([]string{"assets/linked.aeart"})
	if notice, ok := ed.LastNotice(); ok && notice.Level == context.LevelError {
		t.Fatal(notice.Text)
	}
	artID, ok := ed.Pipeline.Lookup("assets/linked.aeart")
	if !ok || artID == source {
		t.Fatal("linked source was not created independently")
	}
	doc := ed.Docs.AssetDocument(artID)
	if doc == nil {
		t.Fatal("linked drawing did not open")
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("linked drawing has no Art Studio model")
	}
	meta, ok := ed.Pipeline.MetaOf(source)
	if !ok {
		t.Fatal("original image metadata disappeared")
	}
	link, err := artsource.TextureLinkOf(&meta)
	if err != nil || link == nil || len(link.Frames) != 2 || model.Session.Frame != link.Frames[1].Frame {
		t.Fatalf("selected source frame was not opened: %v", err)
	}
	if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, B: 255, A: 255}}}}); err != nil {
		t.Fatal(err)
	}
	if err := ed.EditSourceArt(frames[0]); err != nil {
		t.Fatal(err)
	}
	if ed.Docs.AssetDocument(artID) != doc || !doc.Dirty() || model.Session.Frame != link.Frames[0].Frame {
		t.Fatal("switching frames lost the canonical dirty document")
	}
	if err := ed.EditSourceArt(frames[1]); err != nil {
		t.Fatal(err)
	}
	painted, _, err := model.Capture(model.Session.Frame, art.Albedo)
	if err != nil || painted.NRGBAAt(1, 1) != (color.NRGBA{R: 255, B: 255, A: 255}) {
		t.Fatalf("dirty pixels did not survive frame switching: %v", err)
	}
	if err := ed.UndoAsset(doc); err != nil {
		t.Fatal(err)
	}
	if err := ed.RedoAsset(doc); err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(doc); err != nil {
		t.Fatal(err)
	}
	batch, err := ed.Pipeline.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("saved drawing import failed: batch=%+v error=%v", batch, err)
	}
	if got, err := ed.Pipeline.SourceBytes("assets/linked.png"); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original PNG was changed: %v", err)
	}
}

func TestLinkedSourceCreationGuardsConcurrentWork(t *testing.T) {
	for _, scenario := range []string{"dirty metadata", "read-only metadata", "external PNG", "external sidecar", "target appeared", "closed document"} {
		t.Run(scenario, func(t *testing.T) {
			ed, source, frames, _ := linkedArtEditor(t)
			askLegacyLinkedSource(t, ed, frames[1])
			prompt := linkedSourcePrompt(t, ed)
			doc := ed.Docs.AssetDocument(source)
			target := filepath.Join(ed.Project.Root, "assets/linked.aeart")
			switch scenario {
			case "dirty metadata":
				doc.Metadata.Code.Replace(0, 0, "// owner work\n")
			case "read-only metadata":
				doc.Metadata.ReadOnly = true
			case "external PNG":
				if err := os.WriteFile(filepath.Join(ed.Project.Root, "assets/linked.png"), []byte("external image"), 0600); err != nil {
					t.Fatal(err)
				}
			case "external sidecar":
				if err := os.WriteFile(filepath.Join(ed.Project.Root, "assets/linked.png.meta"), []byte("external metadata"), 0600); err != nil {
					t.Fatal(err)
				}
			case "target appeared":
				if err := os.WriteFile(target, []byte("another drawing"), 0600); err != nil {
					t.Fatal(err)
				}
			case "closed document":
				if err := ed.Docs.CloseAsset(source, false); err != nil {
					t.Fatal(err)
				}
			}
			prompt.Submit([]string{"assets/linked.aeart"})
			if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
				t.Fatalf("unsafe source creation was not rejected: %+v", notice)
			}
			if scenario == "target appeared" {
				if data, err := os.ReadFile(target); err != nil || string(data) != "another drawing" {
					t.Fatal("another drawing was overwritten", err)
				}
			} else if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("unsafe source creation left a target: %v", err)
			}
		})
	}
}

func TestOpeningLegacyLinkedSourceWarnsWithoutChangingFiles(t *testing.T) {
	ed, source, frames, original := linkedArtEditor(t)
	askLegacyLinkedSource(t, ed, frames[1])
	linkedSourcePrompt(t, ed).Submit([]string{"assets/linked.aeart"})
	meta, ok := ed.Pipeline.MetaOf(source)
	if !ok {
		t.Fatal("linked PNG metadata missing")
	}
	link, err := artsource.TextureLinkOf(&meta)
	if err != nil || link == nil || link.LayoutSHA256 == "" {
		t.Fatalf("new link lacks its geometry guard: %+v, %v", link, err)
	}
	before, err := ed.Pipeline.SourceBytes("assets/linked.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the compatibility branch without changing the test project's
	// metadata: only the parsed link passed to the opener is made legacy-like.
	link.LayoutSHA256 = ""
	if err := openLinkedArtFrame(ed, link, frames[1]); err != nil {
		t.Fatal(err)
	}
	warning := i18n.Tr("editor.art.legacy_link_geometry", "This older PNG link cannot detect slicing changes. Keep its source rectangles unchanged and review them before importing.")
	if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelWarn || notice.Text != warning {
		t.Fatalf("legacy geometry limitation was not surfaced: %+v", notice)
	}
	after, err := ed.Pipeline.SourceBytes("assets/linked.png.meta")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("opening a legacy link changed metadata: %v", err)
	}
	png, err := ed.Pipeline.SourceBytes("assets/linked.png")
	if err != nil || !bytes.Equal(original, png) {
		t.Fatalf("opening a legacy link changed PNG bytes: %v", err)
	}
}
