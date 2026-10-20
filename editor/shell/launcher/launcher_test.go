package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"aego/editor/core/state"
	"aego/project"
)

func TestCreateKeepsChosenWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	l := New(Options{
		Templates:  []Template{{ID: "2d", Name: "2D"}},
		Workspaces: []Choice{{ID: "design", Title: "Design"}, {ID: "programming", Title: "Programming"}},
		Create: func(dir, template, module, example string) (string, error) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", err
			}
			return dir, os.WriteFile(filepath.Join(dir, project.FileName), []byte("project version=1 {\n}\n"), 0o644)
		},
	})
	if l.Tab() != TabNew {
		t.Fatal("without recent projects the launcher starts on New Project")
	}
	l.SetWorkspace("programming")
	if _, err := l.Create(root, "2d", "", ""); err != nil {
		t.Fatal(err)
	}
	st, err := state.Load(root)
	if err != nil || st.Workspace != "programming" {
		t.Fatalf("workspace = %q, err %v", st.Workspace, err)
	}
	if got, ok := l.Result(); !ok || got != root {
		t.Fatalf("result = %q", got)
	}
}
