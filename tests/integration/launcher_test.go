package integration

import (
	"os"
	"path/filepath"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/shell/launcher"
	"aego/project"
)

func TestLauncherCreatesAndOpens(t *testing.T) {
	recent := filepath.Join(t.TempDir(), "aego", "recent")
	cfg := edit.Config{Headless: true, NoWatch: true, Logger: log.Nop(), Recent: recent}
	ls, err := edit.OpenLauncher(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ls.Step()
	root, err := ls.UI.Create(filepath.Join(t.TempDir(), "game"), "2d", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for ls.Step() {
	}
	ls.Close()
	if got, ok := ls.UI.Result(); !ok || got != root {
		t.Fatalf("result = %q %v", got, ok)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, cfg)
	if err != nil {
		t.Fatal(err)
	}
	s.Step()
	if s.App.Context().Doc() == nil {
		t.Fatal("main scene did not open")
	}
	s.Close()
	list, err := launcher.LoadRecent(recent)
	if err != nil || len(list) != 1 || list[0].Path != root || list[0].Missing {
		t.Fatalf("recent = %+v, %v", list, err)
	}
	os.RemoveAll(root)
	list, _ = launcher.LoadRecent(recent)
	if !list[0].Missing {
		t.Fatal("removed project must be shown as missing")
	}
}
