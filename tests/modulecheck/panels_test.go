package modulecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPanelsUseServices(t *testing.T) {
	root := filepath.Join(repoRoot(t), "editor", "panels")
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := filepath.Glob(filepath.Join(root, d.Name(), "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "context.Editor") {
				t.Errorf("%s uses context.Editor", f)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no panel sources found")
	}
}
