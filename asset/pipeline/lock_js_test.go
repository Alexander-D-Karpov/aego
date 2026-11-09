//go:build js

package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSIndexGuardDoesNotAuthorizeHostWrites(t *testing.T) {
	dir := t.TempDir()
	unlock, owner := acquireIndexLock(dir)
	defer unlock()
	if owner {
		t.Fatal("browser claimed a host index lock")
	}
	if _, err := os.Lstat(filepath.Join(dir, indexLock)); !os.IsNotExist(err) {
		t.Fatalf("browser created a host PID marker: %v", err)
	}
}
