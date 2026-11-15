//go:build linux

package pipeline

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadExternalFileDoesNotBlockOnNamedPipe(t *testing.T) {
	name := filepath.Join(t.TempDir(), "source.ase")
	if err := syscall.Mkfifo(name, 0o600); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := ReadExternalFile(name, 1024); result <- err }()
	select {
	case err := <-result:
		if !errors.Is(err, ErrExternalFileLimit) {
			t.Fatalf("named pipe was not rejected: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reading a named pipe blocked the caller")
	}
}
