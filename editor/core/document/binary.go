package document

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// BinarySource tracks disk ownership for an existing AssetDocument. It does not
// own pixels, tabs or history. Structured editors use it instead of bypassing
// the Manager, coercing a ZIP into ScriptDocument, or writing dirty files directly.
type BinarySource struct {
	manager *Manager
	doc     *AssetDocument
	hash    [32]byte
	limit   int64
}

func (m *Manager) OpenBinarySource(d *AssetDocument, limit int64) (*BinarySource, []byte, error) {
	if d == nil || !validProjectPath(d.Path) || limit < 1 {
		return nil, nil, fmt.Errorf("invalid binary document source")
	}
	s := &BinarySource{manager: m, doc: d, limit: limit}
	data, err := s.read()
	if err != nil {
		return nil, nil, err
	}
	s.hash = sha256.Sum256(data)
	return s, data, nil
}
func (s *BinarySource) read() ([]byte, error) {
	f, err := s.manager.OpenSource(s.doc.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, s.limit+1))
	if err == nil && int64(len(b)) > s.limit {
		err = fmt.Errorf("binary source exceeds size limit")
	}
	return b, err
}
func (s *BinarySource) SHA256() string { return hex.EncodeToString(s.hash[:]) }
func (s *BinarySource) Check() error {
	b, err := s.read()
	if err != nil {
		return fmt.Errorf("source changed or removed; resolve before saving: %w", err)
	}
	if sha256.Sum256(b) != s.hash {
		return fmt.Errorf("source changed externally; resolve before saving: %s", s.doc.Path)
	}
	return nil
}

// Commit is called by art.Document.Publish on the UI thread. It intentionally
// does not emit callbacks under the art document lock. NotifySaved is separate.
func (s *BinarySource) Commit(data []byte) error {
	if !s.doc.Capabilities.Has(EditSource) {
		return fmt.Errorf("source is read-only")
	}
	if int64(len(data)) > s.limit {
		return fmt.Errorf("binary source exceeds size limit")
	}
	if err := s.Check(); err != nil {
		return err
	}
	if err := WriteAtomic(s.manager.Abs(s.doc.Path), data); err != nil {
		return err
	}
	s.hash = sha256.Sum256(data)
	return nil
}
func (s *BinarySource) NotifySaved() {
	s.manager.emit(Event{Kind: EvSourceSaved, Doc: s.doc.ID, Path: s.doc.Path})
}

func (m *Manager) assetRecoveryPath(d *AssetDocument) string {
	return filepath.Join(m.recoveryDir(), "asset-"+d.Asset.String()+".recovery")
}
func (m *Manager) WriteAssetRecovery(d *AssetDocument) error {
	if d.Binding.Recovery == nil || !d.Dirty() {
		return nil
	}
	data, err := d.Binding.Recovery()
	if err != nil {
		return err
	}
	name := m.assetRecoveryPath(d)
	return m.writeRecoveryFile(name, data)
}

// TrackRestoredAssetRecovery remembers the exact copy the user chose to
// restore, so subsequent discard/save never removes a different recovery.
func (m *Manager) TrackRestoredAssetRecovery(d *AssetDocument, data []byte) {
	m.trackRecovery(m.assetRecoveryPath(d), data)
}

func (m *Manager) ClearOwnedAssetRecovery(d *AssetDocument) error {
	return m.discardOwnedRecovery(m.assetRecoveryPath(d))
}
func (m *Manager) ReadAssetRecovery(d *AssetDocument, limit int64) ([]byte, error) {
	f, err := os.Open(m.assetRecoveryPath(d))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(b)) > limit {
		return nil, fmt.Errorf("recovery exceeds size limit")
	}
	return b, err
}
func (m *Manager) ClearAssetRecovery(d *AssetDocument) error {
	err := os.Remove(m.assetRecoveryPath(d))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
