package integration

import (
	"os"
	"path/filepath"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/scene"
)

func TestRestartStatePreservesTheScene(t *testing.T) {
	p := newRotatorProject(t)
	scenePath := filepath.Join(p.ScenesDir(), "main.aescene")
	app, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop(), ScenePath: scenePath}, rotatorModule{})
	if err != nil {
		t.Fatal(err)
	}
	driver := app.Driver()
	for i := 0; i < 60; i++ {
		app.Engine.Step(driver)
	}
	player := app.Scene.Find(app.Scene.Root(), "Player")
	advanced := app.Scene.Local(player).Rotation
	if advanced == 0 {
		t.Fatal("rotation did not advance before the snapshot")
	}

	statePath := run.StatePath(p.Root)
	app.SaveStateAndQuit(statePath)
	if app.Engine.Step(driver) {
		t.Fatal("engine kept running after SaveStateAndQuit")
	}
	app.Close()
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file was not written: %v", err)
	}

	restored, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop(), ScenePath: statePath}, rotatorModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	rp := restored.Scene.Find(restored.Scene.Root(), "Player")
	if rp.IsZero() {
		t.Fatal("player missing after restore")
	}
	if got := restored.Scene.Local(rp).Rotation; got != advanced {
		t.Fatalf("restored rotation %v, want the snapshot's %v", got, advanced)
	}
	if restored.Scene.ID(rp) != app.Scene.ID(player) {
		t.Fatal("node ids changed across the restart")
	}
	if _, ok := scene.Get[Rotator](restored.Scene, rp); !ok {
		t.Fatal("component lost across the restart")
	}
}

func TestRestartStateKeepsUnknownComponents(t *testing.T) {
	p := newRotatorProject(t)
	scenePath := filepath.Join(p.ScenesDir(), "main.aescene")
	app, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop(), ScenePath: scenePath})
	if err != nil {
		t.Fatal(err)
	}
	statePath := run.StatePath(p.Root)
	app.SaveStateAndQuit(statePath)
	app.Close()

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(string(data), "game.Rotator") || !containsString(string(data), "speed=1.5") {
		t.Fatalf("a component from a removed module did not survive the restart:\n%s", data)
	}
}
