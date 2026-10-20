package crash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundleHidesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "game")
	dir, err := Write(Bundle{Root: root, Kind: "panic", Where: "panel inspector", Panic: "boom in " + root, Stack: []byte(root + "/x.go:1")})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"info.aetext", "go.stack"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), home) || !strings.Contains(string(data), "~/game") {
			t.Fatalf("%s is not sanitized: %s", f, data)
		}
	}
}
