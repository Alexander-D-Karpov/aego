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
	if info, err := os.Stat(file); err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("recovery copy is not private: %v, %v", info, err)
	}
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
}
