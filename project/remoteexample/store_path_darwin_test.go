package remoteexample

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinSystemTempAliasDoesNotAllowNestedSymlinks(t *testing.T) {
	canonical, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(canonical, "/private/var/") {
		t.Skip("temporary directory is not under the standard macOS /var alias")
	}
	alias := strings.TrimPrefix(canonical, "/private")
	store, err := NewStore(filepath.Join(alias, "store"), Limits{})
	if err != nil || store.root != filepath.Join(canonical, "store") {
		t.Fatalf("system alias was not safely normalized: %+v, %v", store, err)
	}
	link := filepath.Join(canonical, "user-link")
	if err := os.Symlink(canonical, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(filepath.Join(alias, "user-link", "store2"), Limits{}); err == nil {
		t.Fatal("nested user-controlled symlink was accepted")
	}
}
