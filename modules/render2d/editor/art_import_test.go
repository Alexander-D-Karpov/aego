package editor

import (
	"bytes"
	stdctx "context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/animclip"
	"aego/asset/importers/animctrl"
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

func importTestEditor(t *testing.T) (*context.Editor, *command.Registry, *project.Project) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Aseprite import", false)
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
	for _, importer := range []pipeline.Importer{animclip.New(), animctrl.New()} {
		if err := importers.Register(importer); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, importers, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	commands := command.New()
	ed := &context.Editor{Project: p, Pipeline: pl, Docs: document.NewManager(document.Options{Root: p.Root}), Commands: commands, UI: core.New(nil)}
	ws := workspace.New(p.Root, nil)
	r := ext.New(ext.Deps{Editor: ed, Workspace: ws, Commands: commands})
	if err := r.Run("render2d", imageHooks); err != nil {
		t.Fatal(err)
	}
	return ed, commands, p
}

func importTestSource(t *testing.T, name, extension string) (string, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "art", "testdata", "aseprite", "upstream", name+".aseprite"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), name+extension)
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return source, data
}

func submitImport(t *testing.T, ed *context.Editor, commands *command.Registry, source, target string) {
	t.Helper()
	if err := commands.Run("asset.import_aseprite"); err != nil {
		t.Fatal(err)
	}
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Submit == nil || len(q[0].Fields) != 2 {
		t.Fatalf("import command did not show its two-path prompt: %+v", q)
	}
	q[0].Submit([]string{source, target})
}

func TestAsepriteImportCommandOpensEditableDocumentAndPreservesSource(t *testing.T) {
	ed, commands, p := importTestEditor(t)
	source, original := importTestSource(t, "tags3", ".ase")
	submitImport(t, ed, commands, source, "assets/hero.aeart")
	if notice, ok := ed.LastNotice(); ok && notice.Level == context.LevelError {
		t.Fatalf("import failed: %s", notice.Text)
	}
	if q := ed.TakeQuestions(); len(q) != 0 {
		t.Fatalf("lossless subset unexpectedly asked for confirmation: %+v", q)
	}
	id, ok := ed.Pipeline.Lookup("assets/hero.aeart")
	if !ok {
		t.Fatal("imported source is missing from the asset index")
	}
	doc := ed.Docs.AssetDocument(id)
	if doc == nil || doc.Handler != artHandlerID {
		t.Fatalf("imported source did not open in Art Studio: %+v", doc)
	}
	model, ok := doc.Binding.Model.(*artedit.Document)
	if !ok {
		t.Fatal("imported source did not bind the shared art document")
	}
	info := model.Session.Document.Info()
	if len(info.Frames) != 12 || len(info.Tags) != 3 {
		t.Fatalf("imported timeline did not reach Art Studio: %+v", info)
	}
	for _, f := range info.Frames {
		frameID := artsource.StableID(id, artsource.FrameName(f.ID, art.Albedo))
		if entry, ok := ed.Pipeline.Index().Lookup(frameID); !ok || entry.Kind != asset.ArtSpriteRegion {
			t.Fatalf("prepared frame %s missing from the actual asset index", f.ID)
		}
	}
	for _, tag := range info.Tags {
		clipID := artsource.StableID(id, artsource.ClipName(tag.ID))
		if entry, ok := ed.Pipeline.Index().Lookup(clipID); !ok || entry.Kind != asset.ArtAnim {
			t.Fatalf("prepared animation for %s missing from the actual asset index", tag.Name)
		}
	}
	before, _, err := model.Capture(info.Frames[0].ID, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Cel: info.Cels[0].ID, Points: []image.Point{{X: 0, Y: 2}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 253, G: 2, B: 207, A: 255}}}})
	if err != nil || !doc.Dirty() || model.History.Cursor() != 1 {
		t.Fatalf("imported source was not editable through shared History: dirty=%v err=%v", doc.Dirty(), err)
	}
	if err = model.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	after, _, err := model.Capture(info.Frames[0].ID, art.Albedo)
	if err != nil || !bytes.Equal(before.Pix, after.Pix) {
		t.Fatalf("undo did not restore imported pixels: %v", err)
	}
	if err := ed.Docs.CloseAsset(id, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := ed.OpenAsset(id)
	if err != nil || reopened.Handler != artHandlerID {
		t.Fatalf("reopening the saved imported source failed: %+v, %v", reopened, err)
	}
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "hero.aeart")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("original .ase was changed: %v", err)
	}
}

func TestAsepriteImportWarningsRequireConfirmationAndGuardProject(t *testing.T) {
	ed, commands, p := importTestEditor(t)
	source, original := importTestSource(t, "tags3x123reps", ".aseprite")
	target := "assets/warnings.aeart"
	checkAbsent := func() {
		t.Helper()
		if _, err := os.Stat(filepath.Join(p.Root, filepath.FromSlash(target))); !os.IsNotExist(err) {
			t.Fatalf("source appeared before confirmation: %v", err)
		}
	}
	submitImport(t, ed, commands, source, target)
	q := ed.TakeQuestions()
	if len(q) != 1 || q[0].Then == nil || !strings.Contains(q[0].Message, "tag-repeat") || !strings.Contains(q[0].Message, "metadata-skipped") {
		t.Fatalf("lossy metadata diagnostics were not shown: %+v", q)
	}
	checkAbsent()
	q[0].Then(1)
	checkAbsent()
	submitImport(t, ed, commands, source, target)
	q = ed.TakeQuestions()
	if len(q) != 1 || q[0].Then == nil {
		t.Fatal("second import did not ask for confirmation")
	}
	oldPipeline := ed.Pipeline
	ed.Pipeline = nil // A pending decision must not publish into a different project.
	q[0].Then(0)
	ed.Pipeline = oldPipeline
	checkAbsent()
	// The user confirmed diagnostics for a particular input snapshot.
	submitImport(t, ed, commands, source, target)
	q = ed.TakeQuestions()
	if len(q) != 1 || q[0].Then == nil {
		t.Fatal("changed-source import did not ask for confirmation")
	}
	if err := os.WriteFile(source, []byte("changed during review"), 0o600); err != nil {
		t.Fatal(err)
	}
	q[0].Then(0)
	checkAbsent()
	if n, ok := ed.LastNotice(); !ok || n.Level != context.LevelError || !strings.Contains(n.Text, "source changed") {
		t.Fatalf("changed Aseprite input was not reported: %+v", n)
	}
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	// A target that appeared while the warning was open must not be replaced.
	concurrent := "assets/concurrent.aeart"
	submitImport(t, ed, commands, source, concurrent)
	q = ed.TakeQuestions()
	if len(q) != 1 || q[0].Then == nil {
		t.Fatal("concurrent import did not ask for confirmation")
	}
	other := filepath.Join(p.Root, "assets", "concurrent.aeart")
	if err := os.WriteFile(other, []byte("someone else's work"), 0o600); err != nil {
		t.Fatal(err)
	}
	q[0].Then(0)
	if got, err := os.ReadFile(other); err != nil || string(got) != "someone else's work" {
		t.Fatalf("delayed confirmation overwrote a new file: %q, %v", got, err)
	}
	submitImport(t, ed, commands, source, target)
	q = ed.TakeQuestions()
	if len(q) != 1 || q[0].Then == nil {
		t.Fatal("third import did not ask for confirmation")
	}
	q[0].Then(0)
	if _, ok := ed.Pipeline.Lookup(target); !ok {
		t.Fatal("confirmed import was not added to the asset index")
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("warning confirmation modified the original file: %v", err)
	}
}

func TestAsepriteImportRejectsSymlinkTargetAndNonRegularSource(t *testing.T) {
	ed, commands, p := importTestEditor(t)
	source, _ := importTestSource(t, "tags3", ".aseprite")
	outside := t.TempDir()
	link := filepath.Join(p.Root, "assets", "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	submitImport(t, ed, commands, source, "assets/outside/escape.aeart")
	if n, ok := ed.LastNotice(); !ok || n.Level != context.LevelError || !strings.Contains(n.Text, "symlink") {
		t.Fatalf("symlink target was not rejected: %+v", n)
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.aeart")); !os.IsNotExist(err) {
		t.Fatal("import wrote outside the project")
	}
	directory := filepath.Join(t.TempDir(), "directory.ase")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	submitImport(t, ed, commands, directory, "assets/from-directory.aeart")
	if n, ok := ed.LastNotice(); !ok || n.Level != context.LevelError || !strings.Contains(n.Text, "regular Aseprite file") {
		t.Fatalf("non-regular input was not rejected: %+v", n)
	}
}

func TestAsepriteImportRejectsUnsupportedAndExistingTarget(t *testing.T) {
	ed, commands, p := importTestEditor(t)
	unsupported, _ := importTestSource(t, "2x2tilemap2x2tile", ".aseprite")
	submitImport(t, ed, commands, unsupported, "assets/no.aeart")
	if n, ok := ed.LastNotice(); !ok || n.Level != context.LevelError || !strings.Contains(n.Text, "tile sets are unsupported") {
		t.Fatalf("unsupported visual data was not reported: %+v", n)
	}
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "no.aeart")); !os.IsNotExist(err) {
		t.Fatal("unsupported input created a source")
	}
	existing := filepath.Join(p.Root, "assets", "existing.aeart")
	const userData = "do not overwrite this file"
	if err := os.WriteFile(existing, []byte(userData), 0o600); err != nil {
		t.Fatal(err)
	}
	source, _ := importTestSource(t, "tags3", ".aseprite")
	submitImport(t, ed, commands, source, "assets/existing.aeart")
	if n, ok := ed.LastNotice(); !ok || n.Level != context.LevelError || !strings.Contains(n.Text, "changed since this edit was prepared") {
		t.Fatalf("existing target was not rejected: %+v", n)
	}
	if data, err := os.ReadFile(existing); err != nil || string(data) != userData {
		t.Fatalf("existing target was overwritten: %q, %v", data, err)
	}
}

func TestAsepriteImportRejectsExistingTargetMetadata(t *testing.T) {
	for _, method := range []string{"command", "drop"} {
		t.Run(method, func(t *testing.T) {
			ed, commands, p := importTestEditor(t)
			source, original := importTestSource(t, "tags3", ".ase")
			const target = "assets/tags3.aeart"
			meta, err := ed.Pipeline.NewSourceMeta(target)
			if err != nil {
				t.Fatal(err)
			}
			otherMeta := pipeline.WriteMeta(meta)
			metaPath := filepath.Join(p.Root, "assets", "tags3.aeart.meta")
			if err := os.WriteFile(metaPath, otherMeta, 0600); err != nil {
				t.Fatal(err)
			}
			if method == "command" {
				submitImport(t, ed, commands, source, target)
				if notice, ok := ed.LastNotice(); !ok || notice.Level != context.LevelError {
					t.Fatalf("orphan metadata was not reported: %+v", notice)
				}
			} else {
				if _, _, err := importDroppedAseprite(ed, source, "assets"); err == nil {
					t.Fatal("drop reused existing target metadata")
				}
			}
			if got, err := os.ReadFile(metaPath); err != nil || !bytes.Equal(got, otherMeta) {
				t.Fatalf("existing target metadata was overwritten: %v", err)
			}
			if _, err := os.Stat(filepath.Join(p.Root, "assets", "tags3.aeart")); !os.IsNotExist(err) {
				t.Fatalf("import created a source for existing target metadata: %v", err)
			}
			if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, original) {
				t.Fatalf("original Aseprite file changed: %v", err)
			}
		})
	}
}
