package document

import (
	"errors"
	"fmt"
	"os"

	"aego/core/ids"
)

// HasAssetRecovery and HasScriptRecovery are presence checks, not recovery
// operations. Even an old, unreadable or symlinked entry must block a delayed
// replacement: it may belong to another session and must never be discarded.
func (m *Manager) HasAssetRecovery(id ids.AssetID) (bool, error) {
	if m == nil || id.IsZero() {
		return false, fmt.Errorf("invalid asset recovery identity")
	}
	return recoveryExists(m.assetRecoveryPath(&AssetDocument{Asset: id}))
}

func (m *Manager) HasScriptRecovery(rel string) (bool, error) {
	if m == nil || !validProjectPath(rel) || Clean(rel) != rel {
		return false, fmt.Errorf("invalid script recovery path")
	}
	return recoveryExists(m.scriptRecoveryFile(rel))
}

func recoveryExists(name string) (bool, error) {
	_, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
