package edit

import (
	"path/filepath"
	"testing"

	"aego/app/projects"
	"aego/editor/shell/launcher"
	"aego/project"
)

func TestLauncherCreatesAndOpens(t *testing.T) {
	recent := filepath.Join(t.TempDir(), "recent")
	ls, err := OpenLauncher(Config{Headless: true, Recent: recent})
	if err != nil {
		t.Fatal(err)
	}
	ls.UI.SetTab(launcher.TabNew)
	dir := filepath.Join(t.TempDir(), "AegoProjects", "game")
	if _, err := ls.UI.Create(dir, projects.Template2D, "", ""); err != nil {
		ls.Close()
		t.Fatal(err)
	}
	for i := 0; i < 3 && ls.Step(); i++ {
	}
	ls.Close()
	root, ok := ls.UI.Result()
	if !ok {
		t.Fatal("the launcher has no result")
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(p, Config{Headless: true, NoWatch: true, Recent: recent})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := 0; i < 3; i++ {
		s.Step()
	}
	if s.App.Context().Docs.Active() == nil {
		t.Fatal("no scene is open after creating the project")
	}
	list, _ := launcher.LoadRecent(recent)
	if len(list) == 0 || list[0].Path != root {
		t.Fatalf("recent list %+v", list)
	}
}
