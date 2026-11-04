package document

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
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
