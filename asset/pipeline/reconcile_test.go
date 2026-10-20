package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/project"
)

type txt struct{}

func (txt) ID() string           { return "txt" }
func (txt) Version() int         { return 1 }
func (txt) Extensions() []string { return []string{".txt"} }
func (txt) Defaults() Settings   { return Settings{} }

func (txt) Import(*ImportContext) (*Result, error) {
	return &Result{Kind: asset.ArtTexture}, nil
}

func fixture(t *testing.T, files ...string) (*project.Project, *ImporterRegistry) {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		writeFile(t, filepath.Join(root, "assets", f), "x")
	}
	imps := NewImporterRegistry()
	if err := imps.Register(txt{}); err != nil {
		t.Fatal(err)
	}
	return &project.Project{Root: root}, imps
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func metaID(t *testing.T, path string) ids.AssetID {
	t.Helper()
	data, err := os.ReadFile(path + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseMeta(data, path)
	if err != nil {
		t.Fatal(err)
	}
	return m.ID
}

func open(t *testing.T, p *project.Project, imps *ImporterRegistry) *Pipeline {
	t.Helper()
	pl, err := Open(p, imps, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	return pl
}

func TestEnsureWritesMissingMeta(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	pl := open(t, p, imps)
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "a.txt.meta")); err != nil {
		t.Fatal("meta was not created")
	}
	if l, _ := pl.MetaIssues(); len(l) != 0 {
		t.Fatalf("issues %v", l)
	}
}

func TestReadOnlyAuditKeepsTree(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	issues, err := Audit(p, imps, MetaReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Kind != MetaMissing {
		t.Fatalf("issues %v", issues)
	}
	for _, f := range []string{"assets/a.txt.meta", ".aego"} {
		if _, err := os.Stat(filepath.Join(p.Root, f)); !os.IsNotExist(err) {
			t.Fatalf("%s was written", f)
		}
	}
	again, _ := Audit(p, imps, MetaReadOnly)
	if again[0].ID != issues[0].ID {
		t.Fatal("derived id is not stable")
	}
}

func TestMetaOutlivesSource(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	pl := open(t, p, imps)
	src := filepath.Join(p.Root, "assets", "a.txt")
	id := metaID(t, src)
	os.Remove(src)
	pl.Scan()
	if _, err := os.Stat(src + MetaSuffix); err != nil {
		t.Fatal("meta was removed with the source")
	}
	if l, _ := pl.MetaIssues(); len(l) != 1 || l[0].Kind != MetaOrphan {
		t.Fatalf("issues %v", l)
	}
	writeFile(t, src, "y")
	pl.Scan()
	if got, _ := pl.Lookup("assets/a.txt"); got != id {
		t.Fatal("asset came back with another id")
	}
}

func TestCopyIsDuplicate(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	pl := open(t, p, imps)
	a := filepath.Join(p.Root, "assets", "a.txt")
	b := filepath.Join(p.Root, "assets", "b.txt")
	id := metaID(t, a)
	data, _ := os.ReadFile(a + MetaSuffix)
	writeFile(t, b, "x")
	writeFile(t, b+MetaSuffix, string(data))
	pl.Scan()
	l, _ := pl.MetaIssues()
	if len(l) != 1 || l[0].Kind != MetaDuplicate || l[0].Path != "assets/b.txt" || l[0].Other != "assets/a.txt" {
		t.Fatalf("issues %v", l)
	}
	if n, err := pl.Repair(l); err != nil || n != 1 {
		t.Fatalf("repair %d %v", n, err)
	}
	if metaID(t, a) != id || metaID(t, b) == id {
		t.Fatal("repair touched the original or kept the duplicate id")
	}
	if l, _ := pl.MetaIssues(); len(l) != 0 {
		t.Fatalf("issues after repair %v", l)
	}
}

func TestMoveKeepsID(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	pl := open(t, p, imps)
	id := metaID(t, filepath.Join(p.Root, "assets", "a.txt"))
	if err := pl.Move(id, "assets/sub/c.txt"); err != nil {
		t.Fatal(err)
	}
	if metaID(t, filepath.Join(p.Root, "assets", "sub", "c.txt")) != id {
		t.Fatal("move changed the id")
	}
	if _, err := os.Stat(filepath.Join(p.Root, "assets", "a.txt.meta")); !os.IsNotExist(err) {
		t.Fatal("old meta is still there")
	}
}

func TestSecondOpenIsShared(t *testing.T) {
	p, imps := fixture(t, "a.txt")
	a, _ := Open(p, imps, nil)
	b, _ := Open(p, imps, nil)
	if a.Shared() || !b.Shared() || b.MetaMode() != MetaReadOnly {
		t.Fatal("second pipeline is not shared")
	}
	b.Close()
	a.Close()
	c, _ := Open(p, imps, nil)
	defer c.Close()
	if c.Shared() {
		t.Fatal("lock was not released")
	}
}
