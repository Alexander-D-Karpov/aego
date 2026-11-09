//go:build !windows

package document

import (
	"os"
	"path"
)

type privateRecoveryDir struct {
	root *os.Root
	name string
}

func createPrivateRecoveryDir(root *os.Root, name string) (*privateRecoveryDir, bool, error) {
	if err := root.Mkdir(name, 0700); err != nil {
		return nil, false, err
	}
	return &privateRecoveryDir{root: root, name: name}, true, nil
}

func (d *privateRecoveryDir) CreateFile(name string) (*os.File, error) {
	return d.root.OpenFile(path.Join(d.name, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

func (d *privateRecoveryDir) Close() error { return nil }
