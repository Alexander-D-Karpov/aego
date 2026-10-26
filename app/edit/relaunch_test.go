package edit

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/project"
)

func TestRelaunchArgs(t *testing.T) {
	cases := []struct {
		verb, target, tab string
		want              []string
	}{
		{"", "/p", "", []string{"/p"}},
		{"", "", "recent", []string{"-launcher", "recent"}},
		{"edit", "/p", "", []string{"edit", "/p"}},
		{"edit", "", "new", []string{"edit", "--launcher", "new"}},
	}
	for _, c := range cases {
		if got := relaunchArgs(c.verb, c.target, c.tab); !slices.Equal(got, c.want) {
			t.Errorf("%q %q %q: %v, want %v", c.verb, c.target, c.tab, got, c.want)
		}
	}
	env := withoutEnv([]string{"A=1", "AEGO_SUPERVISED=1", "B=2"}, "AEGO_SUPERVISED")
	if !slices.Equal(env, []string{"A=1", "B=2"}) {
		t.Fatal(env)
	}
}

func TestResolveArg(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, project.FileName), []byte("project version=1 {\n}\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "scenes"), 0o755)
	scene := filepath.Join(root, "scenes", "main.aescene")
	os.WriteFile(scene, []byte("scene version=1 {\n}\n"), 0o644)
	if dir, file := resolveArg(filepath.Join(root, project.FileName)); dir != root || file != "" {
		t.Fatalf("project.aego: %q %q", dir, file)
	}
	dir, file := resolveArg(scene)
	if dir != filepath.Join(root, "scenes") || file != scene {
		t.Fatalf("scene: %q %q", dir, file)
	}
	if rel, ok := relInside(root, file); !ok || rel != "scenes/main.aescene" {
		t.Fatalf("rel %q %v", rel, ok)
	}
}
