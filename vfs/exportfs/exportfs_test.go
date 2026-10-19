package exportfs

import (
	"os"
	"path/filepath"
	"testing"

	"aego/core/errs"
	"aego/vfs"
)

func TestOnlyListedFilesOpen(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.bin"), []byte("abc"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.bin"), []byte("abc"), 0o644)
	v, err := vfs.New(vfs.Mount{Scheme: "data", Source: New(dir, map[string]int64{"a.bin": 3, "b.bin": 4}), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := v.ReadFile("data://a.bin"); err != nil || string(data) != "abc" {
		t.Fatalf("listed file: %q %v", data, err)
	}
	if _, err := v.ReadFile("data://b.bin"); err == nil {
		t.Fatal("a file with the wrong size opened")
	}
	os.WriteFile(filepath.Join(dir, "c.bin"), []byte("x"), 0o644)
	if _, err := v.ReadFile("data://c.bin"); !errs.Has(err, errs.VFSNotFound) {
		t.Fatalf("an unlisted file opened: %v", err)
	}
	if err := v.WriteFile("data://a.bin", nil); err == nil {
		t.Fatal("export data accepted a write")
	}
}
