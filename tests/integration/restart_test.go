package integration

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/scene"
)

func TestRestartStatePreservesTheScene(t *testing.T) {
	const frameTime = 1.0 / 60
	const frames = 60
	const speed = 1.5 // the speed in rotatorScene
	p := newRotatorProject(t)
	scenePath := filepath.Join(p.ScenesDir(), "main.aescene")
	app, err := run.Open(p, run.Config{Headless: true, FrameTime: frameTime, Logger: log.Nop(), ScenePath: scenePath}, rotatorModule{})
	if err != nil {
		t.Fatal(err)
	}
	appOpen := true
	defer func() {
		if appOpen {
			app.Close()
		}
	}()
	player := app.Scene.Find(app.Scene.Root(), "Player")
	if player.IsZero() {
		t.Fatal("player missing before the snapshot")
	}
	rotator, ok := scene.Get[Rotator](app.Scene, player)
	if !ok || rotator.Speed != speed {
		t.Fatalf("rotator before the snapshot = %v, want speed %v", rotator, speed)
	}
	before := app.Scene.Local(player).Rotation
	if before != 0 {
		t.Fatalf("initial rotation = %v, want 0", before)
	}
	playerID := app.Scene.ID(player)
	driver := app.Driver()
	for i := 0; i < frames; i++ {
		if !app.Engine.Step(driver) {
			t.Fatalf("engine stopped at frame %d: %v", i, app.Engine.Err())
		}
	}
	advanced := app.Scene.Local(player).Rotation
	if advanced == 0 {
		t.Fatal("rotation did not advance before the snapshot")
	}
	// The first engine frame has zero delta; all the others use frameTime.
	wantAdvanced := float64(before) + (frames-1)*speed*frameTime
	if !(math.Abs(float64(advanced)-wantAdvanced) <= 1e-5) {
		t.Fatalf("rotation before snapshot = %v, want %v", advanced, wantAdvanced)
	}

	statePath := run.StatePath(p.Root)
	app.SaveStateAndQuit(statePath)
	if app.Engine.Step(driver) {
		t.Fatal("engine kept running after SaveStateAndQuit")
	}
	app.Close()
	appOpen = false
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file was not written: %v", err)
	}

	restored, err := run.Open(p, run.Config{Headless: true, FrameTime: frameTime, Logger: log.Nop(), ScenePath: statePath}, rotatorModule{})
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
	if restored.Scene.ID(rp) != playerID {
		t.Fatal("node ids changed across the restart")
	}
	rotator, ok = scene.Get[Rotator](restored.Scene, rp)
	if !ok || rotator.Speed != speed {
		t.Fatalf("rotator after restart = %v, want speed %v", rotator, speed)
	}
	restoredDriver := restored.Driver()
	for i := 0; i < 2; i++ {
		if !restored.Engine.Step(restoredDriver) {
			t.Fatalf("restored engine stopped at frame %d: %v", i, restored.Engine.Err())
		}
	}
	wantContinued := float64(advanced) + speed*frameTime
	if got := float64(restored.Scene.Local(rp).Rotation); !(math.Abs(got-wantContinued) <= 1e-5) {
		t.Fatalf("rotation after restart and two frames = %v, want %v", got, wantContinued)
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
