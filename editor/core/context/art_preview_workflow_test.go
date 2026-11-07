package context

import (
	"bytes"
	stdctx "context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"aego/art"
	"aego/asset"
	"aego/asset/importers/artsource"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/artedit"
	"aego/editor/core/document"
	"aego/gpu"
	"aego/gpu/null"
	"aego/project"
	"aego/render/texture"
)

// The device records actual loader uploads; no real GPU or scene view is used.
func TestArtWorkingSourcePreviewUndoStaleAndDiscard(t *testing.T) {
	p, err := project.Create(t.TempDir(), "Art preview", false)
	if err != nil {
		t.Fatal(err)
	}
	drawing, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	source, err := drawing.Encode()
	drawing.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets", "hero.aeart"), source, 0644); err != nil {
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
		t.Fatal("source missing from index")
	}
	f := &metadataFixture{dev: &metadataDevice{Device: null.New(), data: map[gpu.TextureHandle][]byte{}}, deviceGen: 1}
	t.Cleanup(f.dev.Close)
	load := loader.New(f.dev, texture.NewUploader(f.dev), log.Nop())
	if err := load.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(load.Close)
	f.ed = &Editor{Pipeline: pl, Loader: load, Docs: document.NewManager(document.Options{Root: p.Root})}
	if err := f.ed.Docs.Handlers().Register(document.Handler{ID: "art.test", Kinds: []asset.ArtifactKind{asset.ArtTexture}, Capabilities: document.View | document.EditSource | document.EditMetadata}); err != nil {
		t.Fatal(err)
	}
	q := f.ed.metadataService()
	q.submit = func(fn func()) { f.workers = append(f.workers, fn) }
	q.onMain = func(fn func()) { f.main = append(f.main, fn) }
	q.device = func() (uint64, bool) { return f.deviceGen, f.lost }
	t.Cleanup(f.ed.CloseAssetPreviews)
	doc, err := f.ed.OpenAsset(id)
	if err != nil {
		t.Fatal(err)
	}
	model, err := artedit.Bind(f.ed.Docs, doc)
	if err != nil {
		t.Fatal(err)
	}
	baseline := f.pixels(t, doc)
	entry, _ := pl.Entry(id)
	artifact, _ := entry.Artifact(asset.ArtTexture)
	persistentPath := artifact.Path
	stroke := func(x int) {
		t.Helper()
		_, err := model.Execute(stdctx.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: x, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	stroke(1)
	if err := f.ed.ApplyMetadataPreview(id); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	painted := f.pixels(t, doc)
	if bytes.Equal(baseline, painted) || doc.WorkingPreview.Applied == 0 || doc.WorkingPreview.Error != "" {
		t.Fatalf("unsaved pixels did not reach the existing loader: %+v", doc.WorkingPreview)
	}
	if disk, err := os.ReadFile(f.ed.Docs.Abs(doc.Path)); err != nil || !bytes.Equal(disk, source) {
		t.Fatal("working preview wrote to the source file")
	}
	entry, _ = pl.Entry(id)
	artifact, _ = entry.Artifact(asset.ArtTexture)
	if artifact.Path != persistentPath {
		t.Fatal("working preview changed the persistent export index")
	}
	if err := f.ed.UndoAsset(doc); err != nil {
		t.Fatal(err)
	}
	if err := f.ed.ApplyMetadataPreview(id); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if !bytes.Equal(baseline, f.pixels(t, doc)) {
		t.Fatal("Undo did not restore the saved runtime pixels")
	}
	if err := f.ed.RedoAsset(doc); err != nil {
		t.Fatal(err)
	}
	if err := f.ed.ApplyMetadataPreview(id); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	stroke(2) // This supersedes the prepared but not yet published snapshot.
	generation := load.Generation()
	f.publish(t)
	if load.Generation() != generation || !bytes.Equal(baseline, f.pixels(t, doc)) {
		t.Fatal("stale art pixels reached the loader")
	}
	f.ed.PollAssetPreviews()
	f.drain(t)
	if bytes.Equal(baseline, f.pixels(t, doc)) || doc.WorkingPreview.Error != "" {
		t.Fatalf("latest art preview did not recover: %+v", doc.WorkingPreview)
	}
	if err := f.ed.Docs.CloseAsset(id, true); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if !bytes.Equal(baseline, f.pixels(t, doc)) || len(q.sourceWorking) != 0 {
		t.Fatal("discard did not restore the saved loader generation")
	}
}
