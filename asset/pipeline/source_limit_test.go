package pipeline

import (
	"bytes"
	"os"
	"testing"
)

// A registered importer can request a larger bounded source transaction
// without raising the limit for all source and metadata files.
type largerSourceImporter struct {
	fakeImporter
	limit int64
}

func (f *largerSourceImporter) SourceEditLimit() int64 { return f.limit }

type largerMetaImporter struct{ largerSourceImporter }

func (*largerMetaImporter) Extensions() []string { return []string{".meta"} }

func TestSourceEditLimitIsBoundedAndDoesNotExpandMetadata(t *testing.T) {
	registry := NewImporterRegistry()
	imp := &largerSourceImporter{fakeImporter: fakeImporter{version: 1}, limit: 64 << 20}
	if err := registry.Register(imp); err != nil {
		t.Fatal(err)
	}
	meta := &largerMetaImporter{largerSourceImporter{fakeImporter: fakeImporter{version: 1}, limit: 64 << 20}}
	// Deliberately claim the metadata extension in a separate registry to prove
	// the pipeline's sidecar bound still wins.
	metaRegistry := NewImporterRegistry()
	if err := metaRegistry.Register(meta); err != nil {
		t.Fatal(err)
	}
	pl := newPipeline(t.TempDir(), registry, nil)
	for _, tc := range []struct {
		name, path string
		requested  int64
		want       int64
	}{
		{"larger source", "assets/a.txt", 64 << 20, 64 << 20},
		{"global maximum", "assets/a.txt", 128 << 20, 64 << 20},
		{"smaller bound", "assets/a.txt", 7, 7},
		{"invalid bound falls back", "assets/a.txt", -1, maxSourceBytes},
		{"unrelated format", "assets/a.png", 64 << 20, maxSourceBytes},
		{"metadata", "assets/a.txt.meta", 64 << 20, maxSourceBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imp.limit = tc.requested
			if got := pl.sourceEditLimit(tc.path); got != tc.want {
				t.Fatalf("limit = %d, want %d", got, tc.want)
			}
		})
	}
	if got := newPipeline(t.TempDir(), metaRegistry, nil).sourceEditLimit("assets/a.txt.meta"); got != maxSourceBytes {
		t.Fatalf("metadata importer raised the sidecar limit: %d", got)
	}
}

func TestApplySourcesRejectsImporterBoundBeforeWriting(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	imp := &largerSourceImporter{fakeImporter: fakeImporter{version: 1}, limit: 5}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("123456")}}, false); err == nil {
		t.Fatal("oversized candidate was accepted")
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("rejected candidate changed the source: %q, %v", got, err)
	}
}

func TestApplySourcesSupportsImporterBoundedLargerFileAndHistory(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	imp := &largerSourceImporter{fakeImporter: fakeImporter{version: 1}, limit: 64 << 20}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", batch, err)
	}
	large := bytes.Repeat([]byte("x"), maxSourceBytes+1)
	edits := []SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: large}}
	for _, back := range []bool{false, true, false} {
		batch, err = pl.ApplySources(edits, back)
		if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
			t.Fatalf("publication back=%v: %+v, %v", back, batch, err)
		}
		want := large
		if back {
			want = edits[0].Before
		}
		got, err := pl.SourceBytes("assets/a.txt")
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("bounded source read back=%v: size=%d err=%v", back, len(got), err)
		}
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, large) {
		t.Fatalf("final file differs: size=%d err=%v", len(got), err)
	}
}
