package document

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

// Remember only copies this manager actually wrote, or that the user chose to
// restore. A different or subsequently changed file is not ours to discard.
func (m *Manager) trackRecovery(name string, data []byte) {
	if m.recoveryWritten == nil {
		m.recoveryWritten = make(map[string][32]byte)
	}
	m.recoveryWritten[name] = sha256.Sum256(data)
}

// checkOwnedRecovery checks existing bytes and file identity; it deliberately
// reports a conflict rather than adopting a recovery from another session.
func (m *Manager) checkOwnedRecovery(name string) (bool, error) {
	want, owned := m.recoveryWritten[name]
	st, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !owned {
		return true, fmt.Errorf("an earlier recovery exists; it was not overwritten: %s", name)
	}
	if !st.Mode().IsRegular() {
		return true, fmt.Errorf("recovery file is no longer a regular file: %s", name)
	}
	f, err := os.Open(name)
	if err != nil {
		return true, err
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, f)
	opened, statErr := f.Stat()
	if err := errors.Join(copyErr, statErr, f.Close()); err != nil {
		return true, err
	}
	current, err := os.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(st, opened) || !os.SameFile(opened, current) || !bytes.Equal(h.Sum(nil), want[:]) {
		return true, fmt.Errorf("recovery file changed; it was not overwritten or discarded: %s", name)
	}
	return true, nil
}

func (m *Manager) writeRecoveryFile(name string, data []byte) error {
	if _, err := m.checkOwnedRecovery(name); err != nil {
		return err
	}
	if err := WriteAtomic(name, data); err != nil {
		return err
	}
	m.trackRecovery(name, data)
	return nil
}

func (m *Manager) discardOwnedRecovery(name string) error {
	if _, owned := m.recoveryWritten[name]; !owned {
		return nil
	}
	exists, err := m.checkOwnedRecovery(name)
	if err != nil {
		return err
	}
	if !exists {
		delete(m.recoveryWritten, name)
		return nil
	}
	// The content and inode checks are best-effort external conflict guards,
	// not an atomic compare-and-delete against another process.
	if err := os.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(m.recoveryWritten, name)
	return nil
}
