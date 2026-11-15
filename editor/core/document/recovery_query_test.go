package document

import (
	"os"
	"path/filepath"
	"testing"

	"aego/core/ids"
)

func TestRecoveryPresenceIncludesUnreadableOrDanglingEntries(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir()})
	id := ids.NewAssetID()
	if yes, err := m.HasAssetRecovery(id); err != nil || yes {
		t.Fatalf("unexpected asset recovery: %v, %v", yes, err)
	}
	if err := os.MkdirAll(filepath.Join(m.opt.Root, RecoveryDir), 0700); err != nil {
		t.Fatal(err)
	}
	assetPath := m.assetRecoveryPath(&AssetDocument{Asset: id})
	if err := os.WriteFile(assetPath, []byte("earlier recovery"), 0000); err != nil {
		t.Fatal(err)
	}
	if yes, err := m.HasAssetRecovery(id); err != nil || !yes {
		t.Fatalf("unreadable asset recovery was ignored: %v, %v", yes, err)
	}
	const rel = "assets/example.aeart.meta"
	if err := os.Symlink("missing", m.scriptRecoveryFile(rel)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if yes, err := m.HasScriptRecovery(rel); err != nil || !yes {
		t.Fatalf("dangling script recovery was ignored: %v, %v", yes, err)
	}
	if _, err := m.HasScriptRecovery("../outside"); err == nil {
		t.Fatal("invalid recovery path was accepted")
	}
}
