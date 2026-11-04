package animedit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"aego/editor/core/document"
)

type documentTestFiles struct{ root string }

func (f documentTestFiles) Read(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(f.root, rel))
}
func (f documentTestFiles) Write(rel string, data []byte) error {
	return document.WriteAtomic(filepath.Join(f.root, rel), data)
}

func TestTimelineFileWritesRespectSharedDirtySource(t *testing.T) {
	root := t.TempDir()
	m := document.NewManager(document.Options{Root: root})
	base := documentTestFiles{root}
	const path = "assets/walk.aeanim"
	if err := base.Write(path, []byte("saved animation\n")); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript(path)
	if err != nil {
		t.Fatal(err)
	}
	files := WithDocuments(base, m)
	d.Code.ReplaceSnapshot([]byte("standalone unsaved change\n"), false)
	if err := files.Write(path, []byte("timeline change\n")); err == nil {
		t.Fatal("Timeline overwrote a dirty standalone animation source")
	}
	if raw, _ := base.Read(path); string(raw) != "saved animation\n" {
		t.Fatal("rejected Timeline write still changed the source")
	}
	if err := m.SaveScript(d); err != nil {
		t.Fatal(err)
	}
	if err := files.Write(path, []byte("timeline change\n")); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Code.Bytes(), []byte("timeline change\n")) || d.Dirty() || m.Script(path) != d {
		t.Fatal("Timeline did not update the existing clean Code document")
	}
	d.Conflict = true
	if err := files.Write(path, []byte("conflicting change\n")); err == nil {
		t.Fatal("Timeline ignored an unresolved external conflict")
	}
}
