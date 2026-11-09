package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicReplacesCompleteFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "nested", "project.aego")
	for _, contents := range [][]byte{[]byte("first generation\n"), {}, []byte("replacement\n")} {
		if err := WriteAtomic(name, contents); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(got, contents) {
			t.Fatalf("saved generation = %q, %v; want %q", got, err, contents)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(name))
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(name) {
		t.Fatalf("temporary file left after successful writes: %v, %v", entries, err)
	}
}

func TestWriteAtomicRenameFailureKeepsExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "destination")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "keep.txt")
	if err := os.WriteFile(marker, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(target, []byte("replacement")); err == nil {
		t.Fatal("renaming a file over a nonempty directory succeeded")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "unchanged" {
		t.Fatalf("failed publication changed the target: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "destination" {
		t.Fatalf("temporary file left after failed rename: %v, %v", entries, err)
	}
}
