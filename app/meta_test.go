package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/art"
	"aego/asset/importers/artsource"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/engine/problems"
	"aego/project"
)

func TestMetaProblemsForArtSourcePresence(t *testing.T) {
	const rel = "assets/sprites/dust.aeart"
	p, err := project.Create(t.TempDir(), "Meta problems", false)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer doc.Close()
	source, err := doc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(p.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, source, 0644); err != nil {
		t.Fatal(err)
	}
	registry := pipeline.NewImporterRegistry()
	if err := registry.Register(artsource.New()); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if pl != nil {
			pl.Close()
		}
	}()
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 || len(batch.Imported) != 3 {
		t.Fatalf("source/frame/animation import: %+v, %v", batch, err)
	}
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	service := problems.New()
	service.Add(MetaProblems(pl))
	if list, _ := service.Collect(); len(list) != 0 {
		t.Fatalf("existing source was reported in Problems: %+v", list)
	}
	pl.Close()
	pl = nil

	// The source really disappears while the pipeline is closed. The reporter
	// must retain META004, then invalidate its cache if the source returns.
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	reopened, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	service = problems.New()
	service.Add(MetaProblems(reopened))
	list, missingRev := service.Collect()
	stale, orphan := 0, 0
	for _, diagnostic := range list {
		switch {
		case diagnostic.Code == "META004" && diagnostic.Subject.File == rel:
			stale++
		case diagnostic.Code == "META002" && diagnostic.Subject.File == rel+pipeline.MetaSuffix:
			orphan++
		}
	}
	// Problems intentionally deduplicates source and sub-assets by path.
	if stale != 1 || orphan != 1 || len(list) != 2 {
		t.Fatalf("missing source was not reported: %+v", list)
	}
	if err := os.WriteFile(full, source, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reopened.Scan(); err != nil {
		t.Fatal(err)
	}
	list, restoredRev := service.Collect()
	if len(list) != 0 || restoredRev == missingRev {
		t.Fatalf("Problems retained a stale cached warning after restoration: %+v", list)
	}

	// A present source with a replaced sidecar ID is different from a missing
	// file, but its old indexed identity must still appear in Problems.
	reopened.Close()
	metaPath := full + pipeline.MetaSuffix
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := pipeline.ParseMeta(metaBytes, rel+pipeline.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	meta.ID = ids.NewAssetID()
	if err := os.WriteFile(metaPath, pipeline.WriteMeta(meta), 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := pipeline.Open(p, registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer changed.Close()
	if _, _, err := changed.Scan(); err != nil {
		t.Fatal(err)
	}
	service = problems.New()
	service.Add(MetaProblems(changed))
	list, _ = service.Collect()
	if len(list) != 1 || list[0].Code != "META004" || list[0].Subject.File != rel || !strings.Contains(list[0].Message, "identity") {
		t.Fatalf("Problems masked or misreported the old indexed identity: %+v", list)
	}
}
