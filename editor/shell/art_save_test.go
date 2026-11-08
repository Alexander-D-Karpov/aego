package shell

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/importers/preset"
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

func TestSavedPresetReimportsAndReportsIncompleteBatch(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Preset save", false)
	if err != nil {
		t.Fatal(err)
	}
	const rel = "assets/effect.aepreset"
	original := []byte("prefab version=1 {\n  node \"10000000000000000000000000000001\" name=\"Effect\" {\n    component \"particles.ParticleSystem2D\" version=1 { rate=1 }\n  }\n}\n")
	if err := os.WriteFile(filepath.Join(p.Root, rel), original, 0644); err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(preset.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, registry, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	id, ok := pl.Lookup(rel)
	if !ok {
		t.Fatal("preset was not indexed")
	}
	ed := &edcontext.Editor{Pipeline: pl, Docs: document.NewManager(document.Options{Root: p.Root})}
	a := &App{ctx: ed, consolePanel: console.New(nil)}
	a.attachCode()
	doc, err := ed.Docs.OpenScript(rel)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(original, []byte("rate=1"), []byte("rate=2"), 1)
	doc.Code.ReplaceSnapshot(changed, false)
	if err := ed.Docs.SaveScript(doc); err != nil {
		t.Fatal(err)
	}
	if err := pl.VerifyCurrent(id); err != nil {
		t.Fatalf("saved preset was not reimported: %v", err)
	}
	entry, ok := pl.Entry(id)
	if !ok {
		t.Fatal("saved preset disappeared from index")
	}
	artifact, ok := entry.Artifact(asset.ArtPreset)
	if !ok {
		t.Fatal("saved preset has no artifact")
	}
	compiled, err := pl.Artifacts().ReadFile(artifact.Path)
	if err != nil || !bytes.Contains(compiled, []byte("rate=2")) {
		t.Fatalf("saved preset artifact is stale: %q, %v", compiled, err)
	}
	// The text editor can save arbitrary content. Both a parser failure and an
	// unstable empty source must be visible rather than silently appearing current.
	doc.Code.ReplaceSnapshot([]byte("not a preset"), false)
	if err := ed.Docs.SaveScript(doc); err != nil {
		t.Fatal(err)
	}
	if notice, ok := ed.LastNotice(); !ok || notice.Level != edcontext.LevelError || !strings.Contains(notice.Text, "failed imports") {
		t.Fatalf("failed import was not reported: %+v", notice)
	}
	doc.Code.ReplaceSnapshot(nil, false)
	if err := ed.Docs.SaveScript(doc); err != nil {
		t.Fatal(err)
	}
	if notice, ok := ed.LastNotice(); !ok || notice.Level != edcontext.LevelWarn || !strings.Contains(notice.Text, "pending imports") {
		t.Fatalf("retry import was not reported: %+v", notice)
	}
	if err := pl.VerifyCurrent(id); err == nil {
		t.Fatal("an unstable source was reported as current")
	}
}
