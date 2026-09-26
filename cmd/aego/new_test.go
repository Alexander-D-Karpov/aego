package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/engine/bootstrap"
	"aego/project"
)

func TestNewProjectRendersPlaceholder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "game")
	if code := cmdNew([]string{dir}); code != 0 {
		t.Fatalf("aego new exited with %d", code)
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.MainScene.IsZero() {
		t.Fatal("main scene was not recorded in project.aego")
	}
	app, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	driver := app.Driver()
	for i := 0; i < 10; i++ {
		if !app.Engine.Step(driver) {
			t.Fatalf("engine stopped at frame %d: %v", i, app.Engine.Err())
		}
	}
	st := app.Runner.Stats()
	if st.Views != 1 {
		t.Fatalf("views = %d", st.Views)
	}
	if st.Sprites != 1 || st.MissingAssets != 0 {
		t.Fatalf("sprites=%d missing=%d", st.Sprites, st.MissingAssets)
	}
}

func TestNewNativeProjectPointsAtEngine(t *testing.T) {
	engine, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "native")
	if code := cmdNew([]string{dir, "--native", "--engine", engine}); code != 0 {
		t.Fatalf("aego new --native exited with %d", code)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	want := "replace aego => " + filepath.ToSlash(engine)
	if !strings.Contains(string(data), want) {
		t.Fatalf("go.mod does not replace aego:\n%s", data)
	}
	if testing.Short() {
		return
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.Build(p, bootstrap.HostTarget(), bootstrap.ModeRun, engine, nil); err != nil {
		t.Fatalf("a fresh native project does not build: %v", err)
	}
}

func TestNewNativeRefusesWithoutEngine(t *testing.T) {
	t.Setenv("AEGO_ROOT", "")
	dir := filepath.Join(t.TempDir(), "native")
	if code := cmdNew([]string{dir, "--native", "--engine", t.TempDir()}); code == 0 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			data, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
			if !strings.Contains(string(data), "replace aego") {
				t.Fatal("native project was created without a replace directive")
			}
		}
	}
}
