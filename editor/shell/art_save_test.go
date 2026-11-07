package shell

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/artedit"
	edcontext "aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/panels/console"
	"aego/project"
)

func TestSavedArtReimportsWithoutFileWatcher(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Art save", false)
	if err != nil {
		t.Fatal(err)
	}
	drawing, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	initial, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "hero.aeart"), initial, 0644); err != nil {
		t.Fatal(err)
	}
	importers := pipeline.NewImporterRegistry()
	if err := importers.Register(artsource.New()); err != nil {
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
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("initial import: %v %+v", err, batch.Failed)
	}
	id, ok := pl.Lookup("assets/hero.aeart")
	if !ok {
		t.Fatal("no source identity")
	}
	ed := &edcontext.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: p.Root})}
	if err := ed.Docs.Handlers().Register(document.Handler{ID: "art.test", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: document.View | document.EditSource | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: ed, consolePanel: console.New(nil)}
	a.attachCode() // Install the same source-saved listener used by the editor.
	doc, err := ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, err := artedit.Bind(ed.Docs, doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(model.Close)
	texturePixels := func() []byte {
		t.Helper()
		entry, ok := pl.Entry(id)
		if !ok {
			t.Fatal("source missing from index")
		}
		artifact, ok := entry.Artifact(asset.ArtTexture)
		if !ok {
			t.Fatal("source has no texture artifact")
		}
		encoded, err := pl.Artifacts().ReadFile(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		texture, err := asset.DecodeTexture(encoded)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Clone(texture.Pixels)
	}
	before := texturePixels()
	frame := model.Session.Frame
	frameID := artsource.StableID(id, artsource.FrameName(frame, art.Albedo))
	if _, ok := pl.Entry(frameID); !ok {
		t.Fatal("initial frame subasset missing")
	}
	_, err = model.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: 1, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ed.SaveAsset(doc); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, texturePixels()) {
		t.Fatal("Save did not reimport the changed pixels without a watcher")
	}
	if _, ok := pl.Entry(frameID); !ok {
		t.Fatal("Save changed the existing frame AssetID")
	}
	if notice, ok := ed.LastNotice(); ok && notice.Level == edcontext.LevelError {
		t.Fatalf("Save reported an import error: %s", notice.Text)
	}
}
