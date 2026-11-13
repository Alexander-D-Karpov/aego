package document

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func addVolatileEdit(t *testing.T, d *ScriptDocument) {
	t.Helper()
	if err := d.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if changed, err := d.Code.ReplaceVolatile(d.Code.Rev(), []byte("private working audio edit")); err != nil || !changed {
		t.Fatalf("cannot prepare volatile edit: %v %v", changed, err)
	}
}

func TestVolatileScriptProtectsSaveReloadAndRecovery(t *testing.T) {
	m, d, before, working := dirtyTransactionFixture(t)
	if count, err := m.writeScriptRecovery(); err != nil || count != 1 {
		t.Fatalf("cannot prepare prior recovery: %d %v", count, err)
	}
	recovery := m.scriptRecoveryFile(d.Path)
	recoveryBefore, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	addVolatileEdit(t, d)
	journal, revision := d.Code, d.Code.Rev()
	if !d.Dirty() || !m.Dirty() {
		t.Fatal("the shared document did not expose volatile dirty state")
	}
	if err := m.SaveScript(d); err == nil {
		t.Fatal("direct SaveScript accepted in-memory structured changes")
	}
	if err := m.SaveAll(); err == nil {
		t.Fatal("Save All accepted in-memory structured changes")
	}
	if tx, err := m.PrepareDirtyScriptTransaction(d, before); err == nil || tx != nil {
		t.Fatal("a text transaction could publish only part of the shared document")
	}
	if err := m.ReloadScript(d); err == nil {
		t.Fatal("ReloadScript silently discarded volatile changes")
	}
	m.CloseScript(d)
	if m.Script(d.Path) != d {
		t.Fatal("CloseScript silently dropped the volatile journal")
	}
	if _, err := m.WriteRecovery(); err == nil {
		t.Fatal("ordinary text recovery claimed to save volatile changes")
	}
	dest := t.TempDir()
	if out, count, err := m.WriteRecoveryCopy(dest); err == nil || out != "" || count != 0 {
		t.Fatal("recovery copy claimed to save volatile changes")
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatalf("refused recovery copy created files: %v %v", entries, err)
	}
	if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, before) {
		t.Fatal("refused save or reload changed the metadata file", err)
	}
	if got, err := os.ReadFile(recovery); err != nil || !bytes.Equal(got, recoveryBefore) {
		t.Fatal("refused recovery replaced existing recovery", err)
	}
	state, attached := d.Code.Volatile()
	if d.Code != journal || d.Code.Rev() != revision || !bytes.Equal(d.Code.Bytes(), working) ||
		!attached || string(state) != "private working audio edit" || !d.Code.CanUndo() {
		t.Fatal("a refused operation lost the shared working journal")
	}
	if err := m.DiscardScript(d); err != nil || d.Code == journal || d.Dirty() {
		t.Fatal("explicit discard could not release the volatile state", err)
	}
	m.CloseScript(d)
	if m.Script(d.Path) != nil {
		t.Fatal("explicitly discarded script remained open")
	}
}

func TestVolatileRecoveryStillWritesOtherDirtyScripts(t *testing.T) {
	m := newManager(t)
	for _, path := range []string{"assets/audio.meta", "scripts/other.aes"} {
		if err := WriteAtomic(m.Abs(path), []byte("saved\n")); err != nil {
			t.Fatal(err)
		}
	}
	audio, err := m.OpenScript("assets/audio.meta")
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.OpenScript("scripts/other.aes")
	if err != nil {
		t.Fatal(err)
	}
	addVolatileEdit(t, audio)
	other.Code.ReplaceSnapshot([]byte("unsaved script\n"), false)
	count, err := m.WriteRecovery()
	if err == nil || count != 1 {
		t.Fatalf("recovery did not report its unsupported part: %d %v", count, err)
	}
	if got, err := os.ReadFile(m.scriptRecoveryFile(other.Path)); err != nil || string(got) != "unsaved script\n" {
		t.Fatal("unsupported audio recovery prevented another script's recovery", err)
	}
	if _, err := os.Stat(m.scriptRecoveryFile(audio.Path)); !os.IsNotExist(err) {
		t.Fatal("text-only recovery was written for volatile audio state", err)
	}
}

func TestVolatileExternalChangeCannotReplaceWorkingJournal(t *testing.T) {
	m, d, _ := imageDocument(t)
	addVolatileEdit(t, d.Metadata)
	journal, revision := d.Metadata.Code, d.Metadata.Code.Rev()
	touchLater(t, m.Abs(d.Metadata.Path), []byte("external metadata\n"))
	reloaded, conflicts := m.CheckExternalScripts()
	if len(reloaded) != 0 || len(conflicts) != 1 || conflicts[0] != d.Metadata || !d.Metadata.Conflict {
		t.Fatal("external metadata did not produce a conflict")
	}
	if err := m.ReloadScript(d.Metadata); err == nil || d.Metadata.Code != journal || d.Metadata.Code.Rev() != revision || !d.Metadata.Code.VolatileDirty() {
		t.Fatal("reload after conflict discarded volatile work")
	}
	if got, err := os.ReadFile(m.Abs(d.Metadata.Path)); err != nil || string(got) != "external metadata\n" {
		t.Fatal("conflict changed external metadata", err)
	}
}

func TestVolatileAssetCloseAndRecoveryRequireExplicitDiscard(t *testing.T) {
	m, d, source := imageDocument(t)
	metadata := bytes.Clone(d.Metadata.Code.Bytes())
	addVolatileEdit(t, d.Metadata)
	called := false
	d.Binding.Recovery = func() ([]byte, error) { called = true; return []byte("incomplete"), nil }
	if !d.Dirty() || !d.CanUndo() {
		t.Fatal("volatile change is absent from the asset's shared document")
	}
	if err := m.SaveAsset(d); err == nil {
		t.Fatal("asset Save ignored volatile state")
	}
	if err := m.CloseAsset(d.Asset, false); err == nil {
		t.Fatal("ordinary asset close discarded volatile state")
	}
	if err := m.WriteAssetRecovery(d); err == nil || called {
		t.Fatal("direct asset recovery claimed to preserve unsupported volatile state")
	}
	if _, err := m.WriteRecovery(); err == nil || called {
		t.Fatal("asset recovery claimed to preserve unsupported volatile state")
	}
	recovery := m.scriptRecoveryFile(d.Metadata.Path)
	if err := os.MkdirAll(filepath.Dir(recovery), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recovery, []byte("unowned recovery"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.CloseAsset(d.Asset, true); err != nil {
		t.Fatal("explicit discard could not close the asset", err)
	}
	if m.AssetDocument(d.Asset) != nil || m.Script(d.Path+".meta") != nil {
		t.Fatal("explicitly discarded asset retained an invisible volatile script")
	}
	if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, source) {
		t.Fatal("asset discard changed the source", err)
	}
	if got, err := os.ReadFile(m.Abs(d.Path + ".meta")); err != nil || !bytes.Equal(got, metadata) {
		t.Fatal("asset discard changed metadata on disk", err)
	}
	if got, err := os.ReadFile(recovery); err != nil || string(got) != "unowned recovery" {
		t.Fatal("asset discard changed unowned recovery", err)
	}
}

func TestVolatileAssetSaveRefusesBeforeRelatedWrites(t *testing.T) {
	for _, all := range []bool{false, true} {
		name := "asset"
		if all {
			name = "all"
		}
		t.Run(name, func(t *testing.T) {
			m, d, _ := imageDocument(t)
			const sourcePath = "art/related.txt"
			original := []byte("original related source\n")
			if err := WriteAtomic(m.Abs(sourcePath), original); err != nil {
				t.Fatal(err)
			}
			source, err := m.OpenScript(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			d.Source = source
			source.Code.ReplaceSnapshot([]byte("unsaved related source\n"), false)
			addVolatileEdit(t, d.Metadata)
			saved := false
			d.Binding.Dirty = func() bool { return true }
			d.Binding.Save = func() error { saved = true; return nil }
			if all {
				err = m.SaveAll()
			} else {
				err = m.SaveAsset(d)
			}
			if err == nil || saved {
				t.Fatal("save invoked the structured binding despite volatile changes")
			}
			if got, err := os.ReadFile(m.Abs(sourcePath)); err != nil || !bytes.Equal(got, original) {
				t.Fatal("save published a related source before refusing the volatile document", err)
			}
			if !source.Dirty() || !d.Metadata.Code.VolatileDirty() {
				t.Fatal("refused save lost working changes")
			}
		})
	}
}

func TestVolatileSaveRefusesChangesIntroducedByCallbacks(t *testing.T) {
	for _, stage := range []string{"validate", "write"} {
		t.Run(stage, func(t *testing.T) {
			m, d, before, _ := dirtyTransactionFixture(t)
			if err := d.Code.AttachVolatile(); err != nil {
				t.Fatal(err)
			}
			introduce := func() {
				if changed, err := d.Code.ReplaceVolatile(d.Code.Rev(), []byte("new structured edit")); err != nil || !changed {
					t.Fatal("cannot introduce structured edit", err)
				}
			}
			if stage == "validate" {
				d.ValidateSave = func([]byte) error { introduce(); return nil }
			} else {
				m.opt.SourceWrite = func(write func() error) error { introduce(); return write() }
			}
			if err := m.SaveScript(d); err == nil {
				t.Fatal("save accepted volatile changes introduced during its callback")
			}
			if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, before) {
				t.Fatal("save wrote text after a volatile callback edit", err)
			}
			if !d.Code.VolatileDirty() || d.SavedEdits != 0 {
				t.Fatal("refused save lost or marked the volatile edit saved")
			}
		})
	}
}

func TestVolatileRelatedSourceCannotBypassAssetSaveGuard(t *testing.T) {
	m, d, _ := imageDocument(t)
	const sourcePath = "art/related-source.txt"
	before := []byte("saved source\n")
	if err := WriteAtomic(m.Abs(sourcePath), before); err != nil {
		t.Fatal(err)
	}
	source, err := m.OpenScript(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	d.Source = source
	source.Code.ReplaceSnapshot([]byte("unsaved source\n"), false)
	tx, err := m.PrepareDirtyScriptTransaction(source, before)
	if err != nil {
		t.Fatal("cannot prepare the clean transaction fixture", err)
	}
	addVolatileEdit(t, d.Metadata)
	if err := m.SaveScript(source); err == nil {
		t.Fatal("direct source Save ignored related volatile metadata")
	}
	if next, err := m.PrepareDirtyScriptTransaction(source, before); err == nil || next != nil {
		t.Fatal("a new source transaction ignored related volatile metadata")
	}
	if _, _, err := tx.Images(); err == nil {
		t.Fatal("a prepared source transaction survived related volatile metadata")
	}
	if got, err := os.ReadFile(m.Abs(sourcePath)); err != nil || !bytes.Equal(got, before) || !source.Dirty() {
		t.Fatal("refused related-source save changed source or lost work", err)
	}
}

func TestVolatileSaveAllRechecksBeforeSaveCallbacks(t *testing.T) {
	m, d, _ := imageDocument(t)
	d.Metadata.Code.ReplaceSnapshot([]byte("unsaved metadata\n"), false)
	if err := d.Metadata.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	d.Binding.BeforeSave = func() error {
		if changed, err := d.Metadata.Code.ReplaceVolatile(d.Metadata.Code.Rev(), []byte("new structured edit")); err != nil || !changed {
			t.Fatal("cannot introduce structured edit", err)
		}
		return nil
	}
	saved := false
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Save = func() error { saved = true; return nil }
	if err := m.SaveAll(); err == nil || saved || !d.Metadata.Code.VolatileDirty() {
		t.Fatal("Save All did not stop after BeforeSave introduced a volatile edit")
	}
}

func TestVolatileSaveRefusesStateIntroducedAfterWrite(t *testing.T) {
	m, d, _, working := dirtyTransactionFixture(t)
	if count, err := m.writeScriptRecovery(); err != nil || count != 1 {
		t.Fatal("cannot create previous recovery", err)
	}
	recovery := m.scriptRecoveryFile(d.Path)
	recoveryBefore, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	m.opt.SourceWrite = func(write func() error) error {
		if err := write(); err != nil {
			return err
		}
		if changed, err := d.Code.ReplaceVolatile(d.Code.Rev(), []byte("new structured edit")); err != nil || !changed {
			t.Fatal("cannot introduce a late structured edit", err)
		}
		return nil
	}
	if err := m.SaveScript(d); err == nil {
		t.Fatal("save reported success after a late volatile edit")
	}
	// The text write happened before the callback's new edit. A refusal can
	// report that state, but cannot roll back the earlier write atomically.
	if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, working) {
		t.Fatal("unexpected text write during late edit", err)
	}
	if got, err := os.ReadFile(recovery); err != nil || !bytes.Equal(got, recoveryBefore) {
		t.Fatal("late edit caused earlier recovery to be discarded", err)
	}
	if !d.Code.VolatileDirty() || d.SavedEdits != 0 {
		t.Fatal("late edit was lost or the full document was marked saved")
	}
}

func TestVolatileAssetSaveRefusesStateIntroducedBySaveBinding(t *testing.T) {
	m, d, _ := imageDocument(t)
	if err := d.Metadata.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	called := false
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Save = func() error {
		called = true
		if changed, err := d.Metadata.Code.ReplaceVolatile(d.Metadata.Code.Rev(), []byte("new structured edit")); err != nil || !changed {
			t.Fatal("cannot introduce a late structured edit", err)
		}
		return nil
	}
	if err := m.SaveAsset(d); err == nil || !called || !d.Metadata.Code.VolatileDirty() {
		t.Fatal("asset Save reported success after a binding introduced volatile work")
	}
}

func TestVolatileSaveAllReportsStateIntroducedDuringSave(t *testing.T) {
	m, d, _ := imageDocument(t)
	if err := d.Metadata.Code.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	const path = "art/other-script.txt"
	if err := WriteAtomic(m.Abs(path), []byte("saved\n")); err != nil {
		t.Fatal(err)
	}
	other, err := m.OpenScript(path)
	if err != nil {
		t.Fatal(err)
	}
	other.Code.ReplaceSnapshot([]byte("unsaved\n"), false)
	m.opt.SourceWrite = func(write func() error) error {
		if err := write(); err != nil {
			return err
		}
		if changed, err := d.Metadata.Code.ReplaceVolatile(d.Metadata.Code.Rev(), []byte("new structured edit")); err != nil || !changed {
			t.Fatal("cannot introduce a late structured edit", err)
		}
		return nil
	}
	if err := m.SaveAll(); err == nil || !d.Metadata.Code.VolatileDirty() {
		t.Fatal("Save All reported success despite new volatile work")
	}
}

func TestVolatileRecoveryRefusesChangesIntroducedByBinding(t *testing.T) {
	for _, copyRecovery := range []bool{false, true} {
		name := "asset"
		if copyRecovery {
			name = "copy"
		}
		t.Run(name, func(t *testing.T) {
			m, d, _ := imageDocument(t)
			d.Binding.Dirty = func() bool { return true }
			d.Binding.Recovery = func() ([]byte, error) {
				addVolatileEdit(t, d.Metadata)
				return []byte("incomplete recovery"), nil
			}
			if copyRecovery {
				dest := t.TempDir()
				out, count, err := m.WriteRecoveryCopy(dest)
				if err == nil || out != "" || count != 0 {
					t.Fatal("recovery copy accepted a volatile edit introduced by its binding")
				}
				if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
					t.Fatal("recovery copy wrote an incomplete asset", err)
				}
			} else {
				if err := m.WriteAssetRecovery(d); err == nil {
					t.Fatal("asset recovery accepted a volatile edit introduced by its binding")
				}
				if _, err := os.Stat(m.assetRecoveryPath(d)); !os.IsNotExist(err) {
					t.Fatal("asset recovery wrote an incomplete document", err)
				}
			}
			if !d.Metadata.Code.VolatileDirty() {
				t.Fatal("failed recovery lost the new volatile edit")
			}
		})
	}
}
