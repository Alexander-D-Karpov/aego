package pipeline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadExternalFileIsBoundedAndRegular(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "source")
	if err := os.WriteFile(file, []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadExternalFile(file, 4); err != nil || string(data) != "1234" {
		t.Fatalf("bounded file: %q, %v", data, err)
	}
	for _, name := range []string{file, root} {
		if _, err := ReadExternalFile(name, 3); !errors.Is(err, ErrExternalFileLimit) {
			t.Errorf("%s: oversized or non-regular file was accepted: %v", name, err)
		}
	}
	if _, err := ReadExternalFile(file, 0); err == nil {
		t.Fatal("invalid size limit was accepted")
	}
	if _, err := ReadExternalFile(filepath.Join(root, "missing"), 4); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
}
