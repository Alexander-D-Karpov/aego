package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/asset"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

type fakeImporter struct {
	calls   map[string]int
	version int
}

func (f *fakeImporter) ID() string {
	return "fake"
}

func (f *fakeImporter) Version() int {
	return f.version
}

func (f *fakeImporter) Extensions() []string {
	return []string{".txt"}
}

func (f *fakeImporter) Defaults() Settings {
	return Settings{"mode": aetext.Ident("plain")}
}

func (f *fakeImporter) Import(ctx *ImportContext) (*Result, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[ctx.Path]++
	return &Result{
		Kind:      asset.ArtScene,
		Artifacts: []Produced{{Kind: asset.ArtScene, Data: ctx.Source}},
	}, nil
}

func newProject(t *testing.T) *project.Project {
	t.Helper()
	root := t.TempDir()
	p, err := project.Create(root, "Test", false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func write(t *testing.T, p *project.Project, rel, content string) string {
	t.Helper()
	full := filepath.Join(p.Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func newPipeline(t *testing.T, p *project.Project, imp Importer) *Pipeline {
	t.Helper()
	reg := NewImporterRegistry()
	if err := reg.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	return pl
}

func TestScanCreatesMeta(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})

	added, removed, err := pl.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || len(removed) != 0 {
		t.Fatalf("added=%d removed=%d", len(added), len(removed))
	}
	metaData, err := os.ReadFile(src + MetaSuffix)
	if err != nil {
		t.Fatal("meta file was not created")
	}
	m, err := ParseMeta(metaData, "a.txt.meta")
	if err != nil {
		t.Fatal(err)
	}
	if m.Importer != "fake" || m.ID != added[0] {
		t.Fatalf("meta = %+v", m)
	}
	if m.Settings.String("mode", "") != "plain" {
		t.Fatal("defaults were not written into the meta")
	}
}

func TestIDSurvivesRename(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	added, _, _ := pl.Scan()
	id := added[0]

	dst := filepath.Join(p.AssetsDir(), "sub", "b.txt")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, dst); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src+MetaSuffix, dst+MetaSuffix); err != nil {
		t.Fatal(err)
	}

	pl2 := newPipeline(t, p, &fakeImporter{version: 1})
	added2, _, _ := pl2.Scan()
	if len(added2) != 1 || added2[0] != id {
		t.Fatal("moving a file with its meta must keep the asset id")
	}
	if path, ok := pl2.PathOf(id); !ok || path != "assets/sub/b.txt" {
		t.Fatalf("path = %q", path)
	}
}

func TestFileWithoutMetaGetsNewID(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	added, _, _ := pl.Scan()
	first := added[0]

	if err := os.Remove(src + MetaSuffix); err != nil {
		t.Fatal(err)
	}
	pl2 := newPipeline(t, p, &fakeImporter{version: 1})
	added2, _, _ := pl2.Scan()
	if added2[0] == first {
		t.Fatal("a file without meta must get a fresh id")
	}
}

func TestCacheSkipsUnchangedImports(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	imp := &fakeImporter{version: 1}
	pl := newPipeline(t, p, imp)
	pl.Scan()

	if _, err := pl.ImportAll(); err != nil {
		t.Fatal(err)
	}
	if imp.calls["assets/a.txt"] != 1 {
		t.Fatalf("first import ran %d times", imp.calls["assets/a.txt"])
	}
	if _, err := pl.ImportAll(); err != nil {
		t.Fatal(err)
	}
	if imp.calls["assets/a.txt"] != 1 {
		t.Fatal("unchanged asset must not be reimported")
	}

	if err := os.WriteFile(src, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.ImportAll(); err != nil {
		t.Fatal(err)
	}
	if imp.calls["assets/a.txt"] != 2 {
		t.Fatal("changed source must be reimported")
	}
}

func TestSettingsChangeInvalidatesCache(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	imp := &fakeImporter{version: 1}
	pl := newPipeline(t, p, imp)
	pl.Scan()
	pl.ImportAll()

	metaPath := src + MetaSuffix
	data, _ := os.ReadFile(metaPath)
	m, _ := ParseMeta(data, "m")
	m.Settings["mode"] = aetext.Ident("fancy")
	os.WriteFile(metaPath, WriteMeta(m), 0o644)

	pl2 := newPipeline(t, p, imp)
	pl2.Scan()
	pl2.ImportAll()
	if imp.calls["assets/a.txt"] < 2 {
		t.Fatal("changing settings must trigger a reimport")
	}
}

func TestImportWritesIndexAndArtifacts(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	batch, err := pl.ImportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Imported) != 1 || len(batch.Failed) != 0 {
		t.Fatalf("batch = %+v", batch)
	}
	entry := batch.Imported[0]
	art, ok := entry.Artifact(asset.ArtScene)
	if !ok {
		t.Fatal("artifact missing")
	}
	data, err := pl.Artifacts().ReadFile(art.Path)
	if err != nil || string(data) != "hello" {
		t.Fatalf("artifact content = %q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(p.CacheDir(), "index.aetext")); err != nil {
		t.Fatal("index was not written")
	}

	reopened := newPipeline(t, p, &fakeImporter{version: 1})
	if reopened.Index().Len() != 1 {
		t.Fatal("index was not reloaded on open")
	}
}

func TestRemovedSourceMarksMissing(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	added, _, _ := pl.Scan()
	pl.ImportAll()

	os.Remove(src)
	os.Remove(src + MetaSuffix)
	_, removed, err := pl.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != added[0] {
		t.Fatalf("removed = %v", removed)
	}
	e, ok := pl.Index().Lookup(added[0])
	if !ok || !e.Missing {
		t.Fatal("entry must be marked missing, not deleted")
	}
}

func TestWatchCoalesces(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	pl.ImportAll()

	batches := make(chan *Batch, 8)
	stop, err := pl.Watch(150*time.Millisecond, func(b *Batch) {
		batches <- b
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	for i := 0; i < 5; i++ {
		write(t, p, "assets/a.txt", "change"+string(rune('0'+i)))
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case b := <-batches:
		if len(b.Imported) != 1 {
			t.Fatalf("expected one coalesced import, got %d", len(b.Imported))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watcher produced no batch")
	}
	select {
	case b := <-batches:
		t.Fatalf("extra batch: %+v", b)
	case <-time.After(400 * time.Millisecond):
	}
}

func TestDependents(t *testing.T) {
	p := newProject(t)
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	a := ids.NewAssetID()
	b := ids.NewAssetID()
	pl.byID[a] = &record{id: a, path: "assets/a.txt"}
	pl.byID[b] = &record{id: b, path: "assets/b.txt", deps: []ids.AssetID{a}}
	deps := pl.Dependents(a)
	if len(deps) != 1 || deps[0] != b {
		t.Fatalf("dependents = %v", deps)
	}
}

func TestEmptySourceIsRetriedNotFailed(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	imp := &fakeImporter{version: 1}
	pl := newPipeline(t, p, imp)
	pl.Scan()
	pl.ImportAll()

	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	id, _ := pl.Lookup("assets/a.txt")
	batch, err := pl.Import(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Failed) != 0 {
		t.Fatalf("an empty source must not be a failure: %v", batch.Failed)
	}
	if len(batch.Retry) != 1 || batch.Retry[0] != id {
		t.Fatalf("an empty source must be retried: %v", batch.Retry)
	}
	if !batch.Empty() {
		t.Fatal("a batch with only retries must not wake subscribers")
	}
	if imp.calls["assets/a.txt"] != 1 {
		t.Fatal("the importer must not see an unstable source")
	}
}

func TestWatchRecoversFromPartialWrite(t *testing.T) {
	p := newProject(t)
	src := write(t, p, "assets/a.txt", "hello")
	pl := newPipeline(t, p, &fakeImporter{version: 1})
	pl.Scan()
	pl.ImportAll()

	batches := make(chan *Batch, 8)
	stop, err := pl.Watch(100*time.Millisecond, func(b *Batch) { batches <- b })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	os.WriteFile(src, nil, 0o644)
	time.Sleep(250 * time.Millisecond)
	os.WriteFile(src, []byte("final"), 0o644)

	deadline := time.After(3 * time.Second)
	for {
		select {
		case b := <-batches:
			if len(b.Failed) != 0 {
				t.Fatalf("partial write surfaced as a failure: %v", b.Failed)
			}
			if len(b.Imported) == 1 {
				data, _ := pl.Artifacts().ReadFile(b.Imported[0].Artifacts[0].Path)
				if string(data) == "final" {
					return
				}
			}
		case <-deadline:
			t.Fatal("the final content was never imported")
		}
	}
}
