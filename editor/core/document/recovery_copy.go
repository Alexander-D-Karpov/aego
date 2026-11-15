package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"aego/core/ids"
)

// WriteRecoveryCopy exports dirty in-memory documents only after the user
// chooses an existing destination directory. It never replaces the project's
// recovery or adopts/restores one. Existing files are never overwritten.
// If any write fails the returned directory may contain a partial copy; the
// caller must report that it is incomplete, not claim recovery succeeded.
func (m *Manager) WriteRecoveryCopy(directory string) (string, int, error) {
	if m == nil || !filepath.IsAbs(directory) {
		return "", 0, fmt.Errorf("choose an existing absolute backup directory")
	}
	if d := m.volatileRecoveryScript(); d != nil {
		return "", 0, fmt.Errorf("%s has in-memory changes without a recovery format", d.Path)
	}
	for _, d := range m.assets {
		if d.Binding.Dirty != nil && d.Binding.Dirty() && d.Binding.Recovery == nil {
			return "", 0, fmt.Errorf("%s cannot be exported as a recovery copy", d.Path)
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", 0, err
	}
	defer root.Close()
	name := "aego-recovery-" + ids.NewAssetID().String()
	var output string
	var copyDir *privateRecoveryDir
	defer func() {
		if copyDir != nil {
			_ = copyDir.Close()
		}
	}()
	count := 0
	write := func(file string, data []byte) error {
		if copyDir == nil {
			var created bool
			copyDir, created, err = createPrivateRecoveryDir(root, name)
			if created {
				output = filepath.Join(directory, name)
			}
			if err != nil {
				return err
			}
		}
		f, err := copyDir.CreateFile(file)
		if err != nil {
			return err
		}
		n, writeErr := f.Write(data)
		if writeErr == nil && n != len(data) {
			writeErr = io.ErrShortWrite
		}
		var syncErr error
		if runtime.GOOS != "js" && runtime.GOOS != "wasip1" {
			syncErr = f.Sync()
		}
		return errors.Join(writeErr, syncErr, f.Close())
	}
	for _, d := range m.docs {
		if !d.Dirty() {
			continue
		}
		if err := write(filepath.Base(m.recoveryFile(d.Path)), m.guarded(d, recoveryBytes)); err != nil {
			return output, count, err
		}
		count++
	}
	for _, d := range m.assets {
		if d.Binding.Dirty == nil || !d.Binding.Dirty() {
			continue
		}
		data, err := d.Binding.Recovery()
		if err != nil {
			return output, count, err
		}
		if changed := m.volatileRecoveryScript(); changed != nil {
			return output, count, fmt.Errorf("%s has in-memory changes without a recovery format", changed.Path)
		}
		if err := write(filepath.Base(m.assetRecoveryPath(d)), data); err != nil {
			return output, count, err
		}
		count++
	}
	for _, d := range m.scripts {
		if !d.Dirty() {
			continue
		}
		if changed := m.volatileRecoveryScript(); changed != nil {
			return output, count, fmt.Errorf("%s has in-memory changes without a recovery format", changed.Path)
		}
		if err := write(filepath.Base(m.scriptRecoveryFile(d.Path)), d.Code.Bytes()); err != nil {
			return output, count, err
		}
		count++
	}
	if count == 0 {
		return "", 0, fmt.Errorf("there are no unsaved documents to export")
	}
	note := "Aego emergency recovery copies from project: " + m.opt.Root + "\n" +
		"These files are copies of unsaved documents, not original assets. " +
		"Their names correspond to the project's .aego/recovery directory. They are not restored automatically. " +
		"Keep existing recovery files, verify the source project and resolve conflicts before any manual restoration. " +
		"Earlier pending recovery files were not copied by this operation.\n"
	if err := write("README.txt", []byte(note)); err != nil {
		return output, count, err
	}
	// A file flush alone does not persist the directory entries. This is still
	// not a proof of power-loss safety, especially on unsupported platforms.
	if runtime.GOOS != "windows" && runtime.GOOS != "js" && runtime.GOOS != "wasip1" {
		for _, dir := range []string{name, "."} {
			f, err := root.Open(dir)
			if err != nil {
				return output, count, err
			}
			if err := errors.Join(f.Sync(), f.Close()); err != nil {
				return output, count, err
			}
		}
	}
	return output, count, nil
}
