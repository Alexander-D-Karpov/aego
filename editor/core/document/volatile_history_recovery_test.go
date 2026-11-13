package document

import (
	"bytes"
	"os"
	"testing"

	"aego/core/ids"
	"aego/editor/core/code"
)

func prepareHiddenVolatileHistory(t *testing.T, d *ScriptDocument, branch string) {
	t.Helper()
	if err := d.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.Code.ReplaceVolatile(d.Code.Rev(), []byte("private audio edit")); err != nil || !changed {
		t.Fatal("cannot prepare structured state", err)
	}
	switch branch {
	case "redo":
		if !d.Code.Undo() || !d.Code.CanRedo() {
			t.Fatal("cannot prepare structured redo")
		}
	case "undo":
		if changed, err := d.Code.ReplaceVolatile(d.Code.Rev(), nil); err != nil || !changed || !d.Code.CanUndo() {
			t.Fatal("cannot prepare structured undo", err)
		}
	default:
		t.Fatal("unknown branch", branch)
	}
	if d.Code.VolatileDirty() {
		t.Fatal("fixture retained an active structured state")
	}
}

func TestVolatileHiddenHistoryRecoveryRefusesTextOnly(t *testing.T) {
	for _, branch := range []string{"undo", "redo"} {
		t.Run(branch, func(t *testing.T) {
			m, d, disk, _ := dirtyTransactionFixture(t)
			if count, err := m.WriteRecovery(); err != nil || count != 1 {
				t.Fatal("cannot create prior recovery", count, err)
			}
			path := m.scriptRecoveryFile(d.Path)
			previous, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			d.Code.ReplaceSnapshot([]byte("meta { value=9 }\n"), false)
			prepareHiddenVolatileHistory(t, d, branch)
			journal, revision, text := d.Code, d.Code.Rev(), bytes.Clone(d.Code.Bytes())
			if count, err := m.WriteRecovery(); err == nil || count != 0 {
				t.Fatal("text recovery accepted hidden structured history", count, err)
			}
			if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, previous) {
				t.Fatal("refused recovery changed an owned prior copy", err)
			}
			dest := t.TempDir()
			if out, count, err := m.WriteRecoveryCopy(dest); err == nil || out != "" || count != 0 {
				t.Fatal("emergency copy accepted hidden structured history", out, count, err)
			}
			if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
				t.Fatal("refused emergency copy created files", entries, err)
			}
			if d.Code != journal || d.Code.Rev() != revision || !bytes.Equal(d.Code.Bytes(), text) || !d.Dirty() {
				t.Fatal("refused recovery changed the working journal")
			}
			if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, disk) {
				t.Fatal("refused recovery changed the source", err)
			}
			// Save publishes only the current text; it need not destroy the still
			// available in-memory Undo/Redo branch.
			if err := m.SaveScript(d); err != nil {
				t.Fatal("hidden history blocked an explicit text save", err)
			}
			if branch == "redo" && (!d.Code.Redo() || !d.Code.VolatileDirty()) {
				t.Fatal("explicit save lost the in-memory structured redo")
			}
		})
	}
}

func TestVolatileCleanRedoRecoveryKeepsPriorCopy(t *testing.T) {
	m, d, disk, _ := dirtyTransactionFixture(t)
	if count, err := m.WriteRecovery(); err != nil || count != 1 {
		t.Fatal("cannot create prior recovery", count, err)
	}
	path := m.scriptRecoveryFile(d.Path)
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for d.Dirty() && d.Code.Undo() {
	}
	if d.Dirty() {
		t.Fatal("fixture could not return to clean text")
	}
	prepareHiddenVolatileHistory(t, d, "redo")
	if d.Dirty() {
		t.Fatal("fixture did not return to a clean current state")
	}
	if count, err := m.WriteRecovery(); err == nil || count != 0 {
		t.Fatal("recovery ignored clean state with structured redo", count, err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, previous) {
		t.Fatal("clean structured redo cleared an owned recovery", err)
	}
	if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, disk) {
		t.Fatal("recovery changed the source", err)
	}
}

func TestVolatileHiddenHistoryAssetRecoveryRefusesCallbacks(t *testing.T) {
	for _, introduced := range []bool{false, true} {
		name := "existing"
		if introduced {
			name = "introduced by callback"
		}
		t.Run(name, func(t *testing.T) {
			m, d, source := imageDocument(t)
			metadata := bytes.Clone(d.Metadata.Code.Bytes())
			d.Metadata.Code.ReplaceSnapshot([]byte("working metadata\n"), false)
			calls := 0
			d.Binding.Recovery = func() ([]byte, error) {
				calls++
				if introduced {
					prepareHiddenVolatileHistory(t, d.Metadata, "redo")
				}
				return []byte("partial recovery"), nil
			}
			if !introduced {
				prepareHiddenVolatileHistory(t, d.Metadata, "redo")
			}
			if err := m.WriteAssetRecovery(d); err == nil {
				t.Fatal("asset recovery accepted hidden structured history")
			}
			if (!introduced && calls != 0) || (introduced && calls != 1) {
				t.Fatal("asset recovery called its callback at the wrong time", calls)
			}
			if _, err := os.Stat(m.assetRecoveryPath(d)); !os.IsNotExist(err) {
				t.Fatal("partial asset recovery was written", err)
			}
			if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, source) {
				t.Fatal("asset recovery changed source bytes", err)
			}
			if got, err := os.ReadFile(m.Abs(d.Metadata.Path)); err != nil || !bytes.Equal(got, metadata) {
				t.Fatal("asset recovery changed metadata bytes", err)
			}
		})
	}
}

func TestVolatileHiddenHistoryRecoveryCopyRefusesLateCallback(t *testing.T) {
	m, d, _ := imageDocument(t)
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Recovery = func() ([]byte, error) {
		prepareHiddenVolatileHistory(t, d.Metadata, "redo")
		return []byte("incomplete recovery"), nil
	}
	dest := t.TempDir()
	if out, count, err := m.WriteRecoveryCopy(dest); err == nil || out != "" || count != 0 {
		t.Fatal("recovery copy accepted hidden history introduced by a callback", out, count, err)
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatal("refused recovery copy created files", entries, err)
	}
	if !d.Metadata.Code.CanRedo() || d.Metadata.Code.VolatileDirty() {
		t.Fatal("refused recovery lost hidden redo")
	}
}

func TestVolatileHiddenHistoryStillRecoversOtherScripts(t *testing.T) {
	m, d, _, _ := dirtyTransactionFixture(t)
	prepareHiddenVolatileHistory(t, d, "redo")
	const otherPath = "scripts/other.aes"
	if err := WriteAtomic(m.Abs(otherPath), []byte("saved\n")); err != nil {
		t.Fatal(err)
	}
	other, err := m.OpenScript(otherPath)
	if err != nil {
		t.Fatal(err)
	}
	other.Code.ReplaceSnapshot([]byte("unsaved\n"), false)
	if count, err := m.WriteRecovery(); err == nil || count != 1 {
		t.Fatal("incomplete recovery did not retain the independent script", count, err)
	}
	if got, err := os.ReadFile(m.scriptRecoveryFile(otherPath)); err != nil || string(got) != "unsaved\n" {
		t.Fatal("independent script recovery is missing", err)
	}
}

func TestVolatileHiddenHistoryRecoveryReportsLateOtherAsset(t *testing.T) {
	m := newManager(t)
	// Model an asset-owned journal that is not also in the script list. A later
	// recovery callback mutates it after its own asset has already been visited.
	source := &ScriptDocument{Path: "virtual/first", Code: code.New([]byte("saved"))}
	first := &AssetDocument{Asset: ids.AssetID{Lo: 41}, Source: source}
	second := &AssetDocument{Asset: ids.AssetID{Lo: 42}}
	second.Binding.Dirty = func() bool { return true }
	second.Binding.Recovery = func() ([]byte, error) {
		prepareHiddenVolatileHistory(t, source, "redo")
		return []byte("already written"), nil
	}
	m.assets = append(m.assets, first, second)
	count, err := m.WriteRecovery()
	if err == nil || count != 1 {
		t.Fatal("recovery reported success after another asset acquired structured history", count, err)
	}
	if got, err := os.ReadFile(m.assetRecoveryPath(second)); err != nil || string(got) != "already written" {
		t.Fatal("the prior recovery write was unexpectedly rolled back", err)
	}
	if !source.Code.CanRedo() || source.Code.VolatileDirty() {
		t.Fatal("late structured redo was not retained")
	}
}

func TestVolatileHiddenHistoryRejectsImplicitReload(t *testing.T) {
	for _, branch := range []string{"undo", "redo"} {
		for _, automatic := range []bool{false, true} {
			name := branch + "/direct"
			if automatic {
				name = branch + "/external check"
			}
			t.Run(name, func(t *testing.T) {
				m := newManager(t)
				const path = "assets/audio.meta"
				if err := WriteAtomic(m.Abs(path), []byte("saved metadata\n")); err != nil {
					t.Fatal(err)
				}
				d, err := m.OpenScript(path)
				if err != nil {
					t.Fatal(err)
				}
				prepareHiddenVolatileHistory(t, d, branch)
				journal, revision := d.Code, d.Code.Rev()
				external := []byte("changed on disk\n")
				touchLater(t, m.Abs(path), external)
				if automatic {
					reloaded, conflicts := m.CheckExternalScripts()
					if len(reloaded) != 0 || len(conflicts) != 1 || conflicts[0] != d || !d.Conflict {
						t.Fatal("external check replaced hidden structured history")
					}
				} else if err := m.ReloadScript(d); err == nil {
					t.Fatal("reload accepted hidden structured history")
				}
				if d.Code != journal || d.Code.Rev() != revision || !d.Code.HasVolatileHistory() {
					t.Fatal("refused reload changed the shared journal")
				}
				if got, err := os.ReadFile(m.Abs(path)); err != nil || !bytes.Equal(got, external) {
					t.Fatal("refused reload changed the external source", err)
				}
				if err := m.DiscardScript(d); err != nil || d.Code == journal || d.Code.HasVolatileHistory() || !bytes.Equal(d.Code.Bytes(), external) {
					t.Fatal("explicit discard did not replace structured history", err)
				}
			})
		}
	}
}
