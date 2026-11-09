//go:build !windows

package document

import (
	"os"
	"testing"
)

func assertPrivateRecovery(t *testing.T, name string, directory bool) {
	t.Helper()
	info, err := os.Stat(name)
	if err != nil || info.IsDir() != directory || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("recovery entry is not private: %s: %v, %v", name, info, err)
	}
}
