package document

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscardRemovedSourceHandlesWrappedFileError(t *testing.T) {
	m := newManager(t)
	const name = "scripts/removed.aes"
	if err := WriteAtomic(m.Abs(name), []byte("saved\n")); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript(name)
	if err != nil {
		t.Fatal(err)
	}
	d.Code.ReplaceSnapshot([]byte("unsaved\n"), false)
	if err := os.Remove(m.Abs(name)); err != nil {
		t.Fatal(err)
	}
	if err := m.DiscardScript(d); err != nil {
		t.Fatal("explicit Discard could not close a removed source", err)
	}
	if d.Dirty() || d.OnDisk || d.Conflict || len(d.Code.Bytes()) != 0 {
		t.Fatal("Discard kept the removed source's dirty journal")
	}
	if _, err := os.Stat(m.Abs(name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Discard recreated a removed source", err)
	}
	m.CloseScript(d)
	if len(m.Scripts()) != 0 {
		t.Fatal("discarded removed source remained open")
	}
}

func TestSavedSourceAccessDoesNotOpenOrReplaceWorkingJournal(t *testing.T) {
	m, d, pixels := imageDocument(t)
	saved := bytes.Clone(d.Metadata.Code.Bytes())
	d.Metadata.Code.ReplaceSnapshot([]byte("meta { strength=9 }\n"), false)
	working := bytes.Clone(d.Metadata.Code.Bytes())
	data, err := m.ReadSource(d.Metadata.Path)
	if err != nil || !bytes.Equal(data, saved) {
		t.Fatal("restore read the working journal instead of saved metadata", err)
	}
	f, err := m.OpenSource(d.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() != int64(len(pixels)) {
		t.Fatal("source stream cannot be bounded before decoding", err)
	}
	data, err = io.ReadAll(f)
	if err != nil || !bytes.Equal(data, pixels) {
		t.Fatal("binary source was not preserved", err)
	}
	if len(m.Scripts()) != 1 || !d.Metadata.Dirty() || !bytes.Equal(d.Metadata.Code.Bytes(), working) {
		t.Fatal("saved source access replaced or duplicated the shared journal")
	}
}

func TestSavedSourceAccessRejectsOutsideProjectAndPreservesMissingError(t *testing.T) {
	m := newManager(t)
	for _, name := range []string{"", ".", "..", "../outside.meta", "/outside.meta", "C:/outside.meta", "art/../../outside.meta"} {
		if _, err := m.OpenSource(name); err == nil {
			t.Fatalf("opened invalid source path %q", name)
		}
	}
	if _, err := m.ReadSource("missing.png.meta"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing source lost its typed error: %v", err)
	}
	if len(m.Scripts()) != 0 {
		t.Fatal("reading a missing source created a document")
	}
}

func TestAcceptSavedScriptPreservesUndoAfterSourceTransaction(t *testing.T) {
	m := newManager(t)
	const name = "assets/linked.png.meta"
	initial := []byte("meta { width=16 }\n")
	before := []byte("meta { width=32 }\n")
	after := []byte("meta { width=32 art_source=asset(\"0123456789abcdef0123456789abcdef\") }\n")
	if err := WriteAtomic(m.Abs(name), initial); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript(name)
	if err != nil {
		t.Fatal(err)
	}
	d.Code.ReplaceSnapshot(before, false)
	if err := m.SaveScript(d); err != nil {
		t.Fatal(err)
	}
	// An unowned recovery file must not be removed or rewritten by adoption.
	recovery := m.scriptRecoveryFile(name)
	if err := os.MkdirAll(filepath.Dir(recovery), 0755); err != nil {
		t.Fatal(err)
	}
	recoveryBytes := []byte("older recovery data\n")
	if err := os.WriteFile(recovery, recoveryBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(m.Abs(name), after); err != nil {
		t.Fatal(err)
	}
	if err := m.AcceptSavedScript(d, before, after); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || d.Conflict || !bytes.Equal(d.Code.Bytes(), after) || !m.scriptMatchesDisk(d) {
		t.Fatal("adopted transaction was not the saved document checkpoint")
	}
	if data, err := os.ReadFile(recovery); err != nil || !bytes.Equal(data, recoveryBytes) {
		t.Fatalf("source adoption changed the recovery file: %v", err)
	}
	d.Code.Undo()
	if !bytes.Equal(d.Code.Bytes(), before) || !d.Dirty() {
		t.Fatal("transaction adoption lost its Undo")
	}
	d.Code.Undo()
	if !bytes.Equal(d.Code.Bytes(), initial) {
		t.Fatal("transaction adoption discarded the earlier document history")
	}
	d.Code.Redo()
	d.Code.Redo()
	if d.Dirty() || !bytes.Equal(d.Code.Bytes(), after) {
		t.Fatal("Redo did not restore the committed checkpoint")
	}
}

func TestAcceptSavedScriptRejectsConcurrentWork(t *testing.T) {
	for _, reason := range []string{"dirty", "external", "external longer", "conflict", "closed", "read-only", "wrong preimage", "invalid postimage"} {
		t.Run(reason, func(t *testing.T) {
			m := newManager(t)
			const name = "assets/linked.png.meta"
			before, after := []byte("meta { value=1 }\n"), []byte("meta { value=2 }\n")
			if err := WriteAtomic(m.Abs(name), before); err != nil {
				t.Fatal(err)
			}
			d, err := m.OpenScript(name)
			if err != nil {
				t.Fatal(err)
			}
			if reason == "invalid postimage" {
				after = []byte{'x', 0, 'y'}
			}
			if err := WriteAtomic(m.Abs(name), after); err != nil {
				t.Fatal(err)
			}
			switch reason {
			case "dirty":
				d.Code.ReplaceSnapshot([]byte("unsaved owner work\n"), false)
			case "external":
				if err := WriteAtomic(m.Abs(name), []byte("external owner work\n")); err != nil {
					t.Fatal(err)
				}
			case "external longer":
				if err := WriteAtomic(m.Abs(name), append(bytes.Clone(after), bytes.Repeat([]byte("external"), 4096)...)); err != nil {
					t.Fatal(err)
				}
			case "conflict":
				d.Conflict = true
			case "closed":
				m.CloseScript(d)
			case "read-only":
				d.ReadOnly = true
			case "wrong preimage":
				// This buffer looks clean and matches the supplied preimage,
				// but that preimage is not the tracked on-disk generation.
				before = []byte("other preimage\n")
				d.Code.ReplaceSnapshot(before, false)
				d.Code.MarkSaved()
			}
			working := bytes.Clone(d.Code.Bytes())
			disk, err := os.ReadFile(m.Abs(name))
			if err != nil {
				t.Fatal(err)
			}
			if err := m.AcceptSavedScript(d, before, after); err == nil {
				t.Fatal("accepted a stale or invalid source transaction")
			}
			current, err := os.ReadFile(m.Abs(name))
			if err != nil || !bytes.Equal(working, d.Code.Bytes()) || !bytes.Equal(disk, current) {
				t.Fatalf("rejected transaction changed owner work: %v", err)
			}
		})
	}
}

func TestAcceptSavedScriptAcceptsTrackedCRLFWithoutChangingPreimage(t *testing.T) {
	m := newManager(t)
	const name = "assets/windows.png.meta"
	before := []byte("meta {\r\n value=1\r\n}\r\n")
	after := []byte("meta {\r\n value=2\r\n}\r\n")
	if err := WriteAtomic(m.Abs(name), before); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(m.Abs(name), after); err != nil {
		t.Fatal(err)
	}
	if err := m.AcceptSavedScript(d, before, after); err != nil {
		t.Fatalf("tracked CRLF source was rejected: %v", err)
	}
	if d.Dirty() || !m.scriptMatchesDisk(d) || !bytes.Equal(d.Code.Bytes(), []byte("meta {\n value=2\n}\n")) {
		t.Fatal("adopted CRLF source has an incorrect clean buffer or disk hash")
	}
	if actual, err := os.ReadFile(m.Abs(name)); err != nil || !bytes.Equal(actual, after) {
		t.Fatalf("adoption rewrote CRLF source bytes: %v", err)
	}
	d.Code.Undo()
	if !bytes.Equal(d.Code.Bytes(), []byte("meta {\n value=1\n}\n")) || !d.Dirty() {
		t.Fatal("Undo lost the normalized preimage")
	}
}
