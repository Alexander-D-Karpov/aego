package memfs

import (
	"io"
	"testing"

	"aego/core/errs"
	"aego/vfs"
)

func TestSource(t *testing.T) {
	v, err := vfs.New(vfs.Mount{Scheme: "data", Source: New(map[string][]byte{"a/b.bin": []byte("hello")}), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	f, err := v.Open("data://a/b.bin")
	if err != nil {
		t.Fatal(err)
	}
	if f.Size() != 5 {
		t.Fatalf("size %d", f.Size())
	}
	if _, err := f.Seek(1, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	rest, _ := io.ReadAll(f)
	if string(rest) != "ello" {
		t.Fatalf("read %q", rest)
	}
	if _, err := v.Open("data://missing"); !errs.Has(err, errs.VFSNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	if err := v.WriteFile("data://a/b.bin", nil); err == nil {
		t.Fatal("game data accepted a write")
	}
}
