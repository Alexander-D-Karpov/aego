package document

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func dirtyTransactionFixture(t *testing.T) (*Manager, *ScriptDocument, []byte, []byte) {
	t.Helper()
	m := newManager(t)
	const path = "assets/transaction.meta"
	before := []byte("meta { value=1 }\r\n")
	after := []byte("meta { value=3 }\n")
	if err := WriteAtomic(m.Abs(path), before); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Code.ReplaceSnapshot([]byte("meta { value=2 }\n"), false)
	d.Code.ReplaceSnapshot(after, false)
	return m, d, before, after
}

func TestDirtyScriptTransactionAdoptsWithoutReplacingHistoryOrRecovery(t *testing.T) {
	m, d, before, after := dirtyTransactionFixture(t)
	journal, revision := d.Code, d.Code.Rev()
	d.ValidateSave = func(data []byte) error {
		// Validation must never be able to mutate the captured postimage.
		data[0] = 'X'
		return nil
	}
	recovery := m.scriptRecoveryFile(d.Path)
	if err := os.MkdirAll(filepath.Dir(recovery), 0o755); err != nil {
		t.Fatal(err)
	}
	recoveryBytes := []byte("unowned recovery\n")
	if err := os.WriteFile(recovery, recoveryBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var events []Event
	unsubscribe := m.Subscribe(func(event Event) { events = append(events, event) })
	defer unsubscribe()
	tx, err := m.PrepareDirtyScriptTransaction(d, before)
	if err != nil {
		t.Fatal(err)
	}
	first, second, err := tx.Images()
	if err != nil || !bytes.Equal(first, before) || !bytes.Equal(second, after) {
		t.Fatal("transaction captured the wrong disk or dirty image")
	}
	first[0], second[0] = 'X', 'Y'
	if a, b, err := tx.Images(); err != nil || !bytes.Equal(a, before) || !bytes.Equal(b, after) {
		t.Fatal("caller mutated the transaction's captured images")
	}
	if !d.Dirty() || d.Code != journal || d.Code.Rev() != revision {
		t.Fatal("preparing a transaction changed the working journal")
	}
	if err := WriteAtomic(m.Abs(d.Path), after); err != nil { // Simulates the caller's already-published transaction.
		t.Fatal(err)
	}
	if err := m.AcceptDirtyScriptTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || d.Code != journal || d.Code.Rev() != revision || d.SavedEdits != 1 || !m.scriptMatchesDisk(d) {
		t.Fatal("adoption did not checkpoint the original shared journal")
	}
	if len(events) != 0 {
		t.Fatalf("adoption unexpectedly requested another save/import: %+v", events)
	}
	if raw, err := os.ReadFile(recovery); err != nil || !bytes.Equal(raw, recoveryBytes) {
		t.Fatalf("adoption changed unowned recovery: %v", err)
	}
	if !d.Code.Undo() || !d.Dirty() || !bytes.Equal(d.Code.Bytes(), []byte("meta { value=2 }\n")) {
		t.Fatal("Undo after adoption lost the preceding dirty edit")
	}
	if !d.Code.Undo() || !bytes.Equal(d.Code.Bytes(), normalizeScriptText(before)) {
		t.Fatal("adoption lost the original undo history")
	}
	if !d.Code.Redo() || !d.Code.Redo() || d.Dirty() || !bytes.Equal(d.Code.Bytes(), after) {
		t.Fatal("Redo did not return to the published checkpoint")
	}
	if raw, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(raw, after) {
		t.Fatalf("Undo or Redo rewrote the published file: %v", err)
	}
	if err := m.AcceptDirtyScriptTransaction(tx); err == nil {
		t.Fatal("accepted the same transaction twice")
	}
}

func TestDirtyScriptTransactionDoesNotClearOwnedRecovery(t *testing.T) {
	m, d, before, after := dirtyTransactionFixture(t)
	if count, err := m.writeScriptRecovery(); err != nil || count != 1 {
		t.Fatal("could not create the test's owned recovery", count, err)
	}
	recovery := m.scriptRecoveryFile(d.Path)
	original, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := m.PrepareDirtyScriptTransaction(d, before)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(m.Abs(d.Path), after); err != nil {
		t.Fatal(err)
	}
	if err := m.AcceptDirtyScriptTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(recovery); err != nil || !bytes.Equal(got, original) {
		t.Fatal("adoption removed or rewrote the owned recovery", err)
	}
	if _, ok := m.recoveryWritten[recovery]; !ok {
		t.Fatal("adoption lost the recovery ownership record")
	}
}

func TestDirtyScriptTransactionRejectsChangedWorkAfterPublication(t *testing.T) {
	for _, reason := range []string{"new edit", "edit and undo", "external same size", "external longer", "conflict", "read-only", "closed", "reload", "validator", "other manager"} {
		t.Run(reason, func(t *testing.T) {
			m, d, before, after := dirtyTransactionFixture(t)
			recovery := m.scriptRecoveryFile(d.Path)
			if err := os.MkdirAll(filepath.Dir(recovery), 0o755); err != nil {
				t.Fatal(err)
			}
			recoveryBytes := []byte("unowned recovery\n")
			if err := os.WriteFile(recovery, recoveryBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			tx, err := m.PrepareDirtyScriptTransaction(d, before)
			if err != nil {
				t.Fatal(err)
			}
			if err := WriteAtomic(m.Abs(d.Path), after); err != nil {
				t.Fatal(err)
			}
			var other *Manager
			switch reason {
			case "new edit":
				d.Code.ReplaceSnapshot([]byte("owner's newer edit\n"), false)
			case "edit and undo":
				d.Code.ReplaceSnapshot([]byte("transient later edit\n"), false)
				if !d.Code.Undo() || !bytes.Equal(d.Code.Bytes(), after) {
					t.Fatal("failed to prepare an ABA edit")
				}
			case "external same size":
				if err := WriteAtomic(m.Abs(d.Path), bytes.Repeat([]byte{'X'}, len(after))); err != nil {
					t.Fatal(err)
				}
			case "external longer":
				if err := WriteAtomic(m.Abs(d.Path), append(bytes.Clone(after), []byte("external")...)); err != nil {
					t.Fatal(err)
				}
			case "conflict":
				d.Conflict = true
			case "read-only":
				d.ReadOnly = true
			case "closed":
				m.CloseScript(d)
			case "reload":
				if err := m.ReloadScript(d); err != nil {
					t.Fatal(err)
				}
			case "validator":
				d.ValidateSave = func([]byte) error { return errors.New("validation changed") }
			case "other manager":
				other = newManager(t)
			}
			working, revision, journal := bytes.Clone(d.Code.Bytes()), d.Code.Rev(), d.Code
			disk, err := os.ReadFile(m.Abs(d.Path))
			if err != nil {
				t.Fatal(err)
			}
			accept := m
			if other != nil {
				accept = other
			}
			if err := accept.AcceptDirtyScriptTransaction(tx); err == nil {
				t.Fatal("accepted a stale dirty transaction")
			}
			raw, err := os.ReadFile(m.Abs(d.Path))
			if err != nil || !bytes.Equal(raw, disk) || d.Code != journal || d.Code.Rev() != revision || !bytes.Equal(d.Code.Bytes(), working) || d.SavedEdits != 0 {
				t.Fatalf("rejection changed disk or owner work: %v", err)
			}
			if raw, err := os.ReadFile(recovery); err != nil || !bytes.Equal(raw, recoveryBytes) {
				t.Fatalf("rejection changed unowned recovery: %v", err)
			}
		})
	}
}

func TestDirtyScriptTransactionPreparationRejectsUnsafePreimages(t *testing.T) {
	for _, reason := range []string{"wrong preimage", "external same size", "external longer", "conflict", "read-only", "closed", "clean", "invalid text", "validator"} {
		t.Run(reason, func(t *testing.T) {
			m, d, before, _ := dirtyTransactionFixture(t)
			switch reason {
			case "wrong preimage":
				before = []byte("not the tracked preimage")
			case "external same size":
				if err := WriteAtomic(m.Abs(d.Path), bytes.Repeat([]byte{'X'}, len(before))); err != nil {
					t.Fatal(err)
				}
			case "external longer":
				if err := WriteAtomic(m.Abs(d.Path), append(bytes.Clone(before), []byte("external")...)); err != nil {
					t.Fatal(err)
				}
			case "conflict":
				d.Conflict = true
			case "read-only":
				d.ReadOnly = true
			case "closed":
				m.CloseScript(d)
			case "clean":
				d.Code.MarkSaved()
			case "invalid text":
				d.Code.ReplaceSnapshot([]byte{'x', 0, 'y'}, false)
			case "validator":
				d.ValidateSave = func([]byte) error { return errors.New("invalid metadata") }
			}
			working, revision := bytes.Clone(d.Code.Bytes()), d.Code.Rev()
			disk, err := os.ReadFile(m.Abs(d.Path))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.PrepareDirtyScriptTransaction(d, before); err == nil {
				t.Fatal("prepared an unsafe dirty transaction")
			}
			raw, err := os.ReadFile(m.Abs(d.Path))
			if err != nil || !bytes.Equal(raw, disk) || d.Code.Rev() != revision || !bytes.Equal(d.Code.Bytes(), working) {
				t.Fatalf("rejected preparation changed owner work: %v", err)
			}
		})
	}
}

func TestDirtyScriptTransactionRejectsValidationThatChangesJournal(t *testing.T) {
	m, d, before, _ := dirtyTransactionFixture(t)
	d.ValidateSave = func([]byte) error {
		d.Code.ReplaceSnapshot([]byte("new owner work\n"), false)
		return nil
	}
	if _, err := m.PrepareDirtyScriptTransaction(d, before); err == nil {
		t.Fatal("accepted a document mutated during validation")
	}
	if !d.Dirty() || !bytes.Equal(d.Code.Bytes(), []byte("new owner work\n")) {
		t.Fatal("failed preparation discarded the validator's edit")
	}
}

func TestDirtyScriptTransactionRejectsMutationDuringAcceptance(t *testing.T) {
	m, d, before, after := dirtyTransactionFixture(t)
	tx, err := m.PrepareDirtyScriptTransaction(d, before)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(m.Abs(d.Path), after); err != nil {
		t.Fatal(err)
	}
	d.ValidateSave = func([]byte) error {
		d.Code.ReplaceSnapshot([]byte("new owner work during acceptance\n"), false)
		return nil
	}
	if err := m.AcceptDirtyScriptTransaction(tx); err == nil {
		t.Fatal("accepted a journal mutated during validation")
	}
	if !d.Dirty() || d.SavedEdits != 0 || !bytes.Equal(d.Code.Bytes(), []byte("new owner work during acceptance\n")) {
		t.Fatal("failed acceptance discarded the owner's newer edit")
	}
	if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, after) {
		t.Fatal("failed acceptance rewrote the already-published disk image", err)
	}
}

func TestDirtyScriptTransactionBoundsImagesBeforeReading(t *testing.T) {
	m, d, before, _ := dirtyTransactionFixture(t)
	tooLarge := bytes.Repeat([]byte{'x'}, maxTransactionScriptBytes+1)
	if _, err := m.PrepareDirtyScriptTransaction(d, tooLarge); err == nil {
		t.Fatal("accepted an oversized preimage")
	}
	d.Code.ReplaceSnapshot(tooLarge, false)
	if _, err := m.PrepareDirtyScriptTransaction(d, before); err == nil {
		t.Fatal("accepted an oversized dirty postimage")
	}
	if !d.Dirty() || !bytes.Equal(d.Code.Bytes(), tooLarge) {
		t.Fatal("rejected oversized image changed the owner's work")
	}
}

func TestDirtyScriptTransactionDistinguishesEmptyFilesFromAbsentPaths(t *testing.T) {
	m := newManager(t)
	const path = "assets/empty.meta"
	if err := WriteAtomic(m.Abs(path), []byte{}); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Code.ReplaceSnapshot([]byte("new content\n"), false)
	if _, err := m.PrepareDirtyScriptTransaction(d, nil); err == nil {
		t.Fatal("treated a nil missing-file preimage as an existing empty file")
	}
	tx, err := m.PrepareDirtyScriptTransaction(d, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	before, after, err := tx.Images()
	if err != nil || before == nil || len(before) != 0 || after == nil || string(after) != "new content\n" {
		t.Fatal("lost the existing empty-file preimage")
	}
	if err := WriteAtomic(m.Abs(path), after); err != nil {
		t.Fatal(err)
	}
	if err := m.AcceptDirtyScriptTransaction(tx); err != nil {
		t.Fatal(err)
	}
	d.Code.ReplaceSnapshot([]byte{}, false)
	tx, err = m.PrepareDirtyScriptTransaction(d, after)
	if err != nil {
		t.Fatal(err)
	}
	_, empty, err := tx.Images()
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatal("treated an empty postimage as a delete operation")
	}
}

func TestDirtyScriptTransactionRechecksBeforeReturningImages(t *testing.T) {
	for _, reason := range []string{"new edit", "edit and undo", "external same size"} {
		t.Run(reason, func(t *testing.T) {
			m, d, before, _ := dirtyTransactionFixture(t)
			tx, err := m.PrepareDirtyScriptTransaction(d, before)
			if err != nil {
				t.Fatal(err)
			}
			switch reason {
			case "new edit":
				d.Code.ReplaceSnapshot([]byte("new owner work\n"), false)
			case "edit and undo":
				d.Code.ReplaceSnapshot([]byte("transient edit\n"), false)
				if !d.Code.Undo() {
					t.Fatal("cannot prepare ABA edit")
				}
			case "external same size":
				if err := WriteAtomic(m.Abs(d.Path), bytes.Repeat([]byte{'X'}, len(before))); err != nil {
					t.Fatal(err)
				}
			}
			working := bytes.Clone(d.Code.Bytes())
			disk, err := os.ReadFile(m.Abs(d.Path))
			if err != nil {
				t.Fatal(err)
			}
			if a, b, err := tx.Images(); err == nil || a != nil || b != nil {
				t.Fatal("returned stale transaction images")
			}
			if got, err := os.ReadFile(m.Abs(d.Path)); err != nil || !bytes.Equal(got, disk) || !bytes.Equal(d.Code.Bytes(), working) {
				t.Fatal("rejected image request changed owner work", err)
			}
		})
	}
}
