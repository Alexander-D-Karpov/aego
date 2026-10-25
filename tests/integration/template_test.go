package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/edit"
	"aego/editor/panels/welcome"
)

// A new 2D project matches Getting started: sidecars for every script, a Move action and the Spinner example.
func TestTemplate2DMatchesGettingStarted(t *testing.T) {
	root, err := edit.CreateProject(filepath.Join(t.TempDir(), "hello"), "2d", "", "")
	if err != nil {
		t.Fatal(err)
	}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && filepath.Ext(p) == ".aes" {
			if _, err := os.Stat(p + ".meta"); err != nil {
				t.Errorf("%s has no .meta", p)
			}
		}
		return nil
	})
	data, err := os.ReadFile(filepath.Join(root, "project.aego"))
	if err != nil || !strings.Contains(string(data), "name=Move") {
		t.Fatalf("project.aego has no Move action: %v", err)
	}
	wrote, err := welcome.Example(filepath.Join(root, "scripts"))
	if err != nil || !wrote {
		t.Fatalf("the Spinner example is not written into a 2D project: %v %v", wrote, err)
	}
}
