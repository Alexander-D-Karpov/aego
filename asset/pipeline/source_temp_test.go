package pipeline

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSourceTemp(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  string
		wantErr bool
	}{
		{name: "unchanged"},
		{name: "already-missing", change: "missing"},
		{name: "replacement-with-identical-bytes", change: "replacement", wantErr: true},
		{name: "symlink-replacement", change: "symlink", wantErr: true},
		{name: "changed-bytes-with-restored-timestamp", change: "contents", wantErr: true},
		{name: "unknown-owner", change: "unknown", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			const name = ".aego-owned.tmp"
			ownedBytes := []byte("owned bytes")
			if err := os.WriteFile(filepath.Join(dir, name), ownedBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			owned, err := root.Lstat(name)
			if err != nil {
				t.Fatal(err)
			}
			want := ownedBytes
			switch tc.change {
			case "missing":
				if err := root.Remove(name); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				// Retain the original inode so the filesystem cannot reuse it.
				if err := root.Rename(name, "original"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), ownedBytes, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(filepath.Join(dir, name), owned.ModTime(), owned.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := root.Rename(name, "original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("original", filepath.Join(dir, name)); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "contents":
				want = []byte("other bytes") // Same length and inode as the owned file.
				if err := os.WriteFile(filepath.Join(dir, name), want, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(filepath.Join(dir, name), owned.ModTime(), owned.ModTime()); err != nil {
					t.Fatal(err)
				}
				current, err := root.Lstat(name)
				if err != nil || !os.SameFile(owned, current) || !owned.ModTime().Equal(current.ModTime()) {
					t.Fatalf("could not reproduce same-inode, same-timestamp edit: %v", err)
				}
			case "unknown":
				owned = nil
			}
			err = removeSourceTemp(root, name, owned, ownedBytes)
			if (err != nil) != tc.wantErr {
				t.Errorf("cleanup error = %v, want error %v", err, tc.wantErr)
			}
			got, err := os.ReadFile(filepath.Join(dir, name))
			if tc.wantErr {
				if err != nil || !bytes.Equal(got, want) {
					t.Errorf("external file was changed or removed: %q, %v", got, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Errorf("owned temporary file remains: %q, %v", got, err)
			}
		})
	}
}

func TestReplaceSourceCleansOnlyItsFailedTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := root.Mkdir("assets", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceSource(root, "assets/source.txt", []byte("candidate")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "assets", "source.txt"))
	if err != nil || string(got) != "candidate" {
		t.Fatalf("successful publication: %q, %v", got, err)
	}
	if err := root.Mkdir("assets/destination", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceSource(root, "assets/destination", []byte("cannot replace a directory")); err == nil {
		t.Fatal("publication over a directory unexpectedly succeeded")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "assets"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "destination" || entries[1].Name() != "source.txt" {
		t.Fatalf("failed publication left unexpected files: %v", entries)
	}
}
