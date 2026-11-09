package document

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	"aego/core/ids"
)

func TestRecoveryCopyIsExplicitPrivateAndDoesNotReplaceEarlierRecovery(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	m := NewManager(Options{Root: root})
	d, err := m.NewTransientAsset(ids.NewAssetID(), "drawing", "art", asset.ArtTexture, View|EditSource)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("current unsaved drawing")
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Recovery = func() ([]byte, error) { return bytes.Clone(want), nil }
	if err := os.MkdirAll(filepath.Join(root, RecoveryDir), 0700); err != nil {
		t.Fatal(err)
	}
	oldName := m.assetRecoveryPath(d)
	old := []byte("older recovery must remain")
	if err := os.WriteFile(oldName, old, 0600); err != nil {
		t.Fatal(err)
	}
	dir, count, err := m.WriteRecoveryCopy(external)
	if err != nil || count != 1 || filepath.Dir(dir) != external {
		t.Fatalf("copy failed: path=%q count=%d error=%v", dir, count, err)
	}
	file := filepath.Join(dir, filepath.Base(oldName))
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("copy lost the working drawing: %q, %v", got, err)
	}
	if got, err := os.ReadFile(oldName); err != nil || !bytes.Equal(got, old) {
		t.Fatalf("the earlier recovery changed: %q, %v", got, err)
	}
	assertPrivateRecovery(t, dir, true)
	assertPrivateRecovery(t, file, false)
	assertPrivateRecovery(t, filepath.Join(dir, "README.txt"), false)
	next, nextCount, err := m.WriteRecoveryCopy(external)
	if err != nil || nextCount != 1 || next == dir {
		t.Fatal("another explicit copy overwrote the first", err)
	}
	if _, _, err := m.WriteRecoveryCopy("relative/path"); err == nil {
		t.Fatal("a relative backup destination was accepted")
	}
}

func TestRecoveryCopyReportsPartialFailure(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir()})
	for i := range 2 {
		d, err := m.NewTransientAsset(ids.NewAssetID(), "drawing", "art", asset.ArtTexture, View|EditSource)
		if err != nil {
			t.Fatal(err)
		}
		d.Binding.Dirty = func() bool { return true }
		d.Binding.Recovery = func() ([]byte, error) {
			if i == 1 {
				return nil, errors.New("cannot serialize the second drawing")
			}
			return []byte("first drawing"), nil
		}
	}
	output, count, err := m.WriteRecoveryCopy(t.TempDir())
	if err == nil || output == "" || count != 1 {
		t.Fatalf("partial failure was reported as success: %q, %d, %v", output, count, err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal("partial copy should be preserved for inspection", err)
	}
	assertPrivateRecovery(t, output, true)
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial copy contents: %v, %v", entries, err)
	}
	partial := filepath.Join(output, entries[0].Name())
	if data, err := os.ReadFile(partial); err != nil || string(data) != "first drawing" {
		t.Fatalf("partial copy was lost: %q, %v", data, err)
	}
	assertPrivateRecovery(t, partial, false)
}

func TestPrivateRecoveryDirectoryAndFilesAreExclusive(t *testing.T) {
	parent := t.TempDir()
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	dir, created, err := createPrivateRecoveryDir(root, "copy")
	if err != nil || !created {
		t.Fatalf("create private directory: created=%v err=%v", created, err)
	}
	defer dir.Close()
	f, err := dir.CreateFile("drawing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("preserve me")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if f, err := dir.CreateFile("drawing"); err == nil {
		f.Close()
		t.Fatal("an existing recovery file was opened for replacement")
	}
	if other, created, err := createPrivateRecoveryDir(root, "copy"); err == nil || created {
		if other != nil {
			other.Close()
		}
		t.Fatalf("an existing recovery directory was adopted: created=%v err=%v", created, err)
	}
	if got, err := os.ReadFile(filepath.Join(parent, "copy", "drawing")); err != nil || string(got) != "preserve me" {
		t.Fatalf("existing copy changed: %q, %v", got, err)
	}
}

func TestRecoveryCopyWithNothingDirtyCreatesNothing(t *testing.T) {
	parent := t.TempDir()
	m := NewManager(Options{Root: t.TempDir()})
	if output, count, err := m.WriteRecoveryCopy(parent); err == nil || output != "" || count != 0 {
		t.Fatalf("empty copy: path=%q count=%d error=%v", output, count, err)
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("empty copy created entries: %v, %v", entries, err)
	}
}

func TestRecoveryCopyRejectsUnsupportedDirtyAssetBeforeWriting(t *testing.T) {
	parent := t.TempDir()
	m := NewManager(Options{Root: t.TempDir()})
	for i := range 2 {
		d, err := m.NewTransientAsset(ids.NewAssetID(), "drawing", "art", asset.ArtTexture, View|EditSource)
		if err != nil {
			t.Fatal(err)
		}
		d.Binding.Dirty = func() bool { return true }
		if i == 0 {
			d.Binding.Recovery = func() ([]byte, error) { return []byte("unsaved"), nil }
		}
	}
	if output, count, err := m.WriteRecoveryCopy(parent); err == nil || output != "" || count != 0 {
		t.Fatalf("unsupported dirty asset: path=%q count=%d error=%v", output, count, err)
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("unsupported asset created entries: %v, %v", entries, err)
	}
}
