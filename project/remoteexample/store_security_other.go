//go:build !windows

package remoteexample

import (
	"fmt"
	"os"
)

func rejectReparse(string) error { return nil }

func newSecureTreeChecker() (func(string, os.FileInfo) error, error) {
	return func(string, os.FileInfo) error { return nil }, nil
}

func secureStoreDirectory(name string, info os.FileInfo) error {
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("example store must be a non-writable-by-others directory: %s", name)
	}
	return nil
}
