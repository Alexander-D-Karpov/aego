package project

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// WriteAtomic writes a complete temporary file before publishing it with a
// rename. On native systems it flushes the temporary file before publication.
// This does not sync the containing directory, provide a compare-and-swap, or
// guarantee power-loss durability of the renamed directory entry.
func WriteAtomic(abs string, data []byte) error {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".aego-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	var syncErr error
	// Browser/WASI storage adapters do not offer a portable native file flush.
	if runtime.GOOS != "js" && runtime.GOOS != "wasip1" {
		syncErr = tmp.Sync()
	}
	if err := errors.Join(syncErr, tmp.Close()); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, abs); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
