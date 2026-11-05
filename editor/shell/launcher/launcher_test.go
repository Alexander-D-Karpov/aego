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

func TestDefaultProjectNameSkipsExistingEntries(t *testing.T) {
	dir := t.TempDir()
	l := New(Options{Home: dir})
	if l.name != "new-game" {
		t.Fatalf("empty location: %q", l.name)
	}
	if err := os.Mkdir(filepath.Join(dir, "new-game"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new-game-2"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := nextProjectName(dir); got != "new-game-3" {
		t.Fatalf("next available name = %q", got)
	}
	if _, err := projectTarget(dir, "new-game"); err == nil {
		t.Fatal("an existing project directory was accepted")
	}
	if _, err := projectTarget(dir, "new-game-2"); err == nil {
		t.Fatal("an existing file was accepted")
	}
	if _, err := projectTarget(dir, "../other"); err == nil {
		t.Fatal("a path was accepted as a project name")
	}
	if target, err := projectTarget(dir, "new-game-3"); err != nil || target != filepath.Join(dir, "new-game-3") {
		t.Fatalf("free target = %q, %v", target, err)
	}
}
