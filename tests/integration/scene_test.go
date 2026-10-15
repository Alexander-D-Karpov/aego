package integration

import (
	"os"
	"path/filepath"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/modules/render2d"
	"aego/project"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/sdk"
)

const rotatorScene = `scene version=1 {
  node "1a2b3c4d5e6f708192a3b4c5d6e7f809" name="Camera" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "engine.Camera" version=1 {
      zoom=64
      viewport=[0, 0, 1, 1]
      mask=-1
      order=0
      clear=true
      clearColor=#0a0d12ff
      scale=free
      virtual=[0, 0]
      active=true
    }
  }
  node "2b3c4d5e6f708192a3b4c5d6e7f8091a" name="Player" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "game.Rotator" version=1 {
      speed=1.5
    }
  }
}
`

type Rotator struct {
	Speed float32 `aego:"speed"`
}

type rotatorSystem struct{}

func (rotatorSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	sdk.Query1[Rotator](s).Each(func(n sdk.NodeHandle, r *Rotator) {
		s.SetRotation(n, s.Local(n).Rotation+r.Speed*dt)
	})
}

type rotatorModule struct{}

func (rotatorModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "game.rotator", Version: "0.1.0"}
}

func (rotatorModule) Register(r *sdk.Registry) error {
	if err := sdk.RegisterComponent[Rotator](r, sdk.Name("game.Rotator")); err != nil {
		return err
	}
	return sdk.RegisterSystem(r, sdk.SystemDesc{
		ID:     "game.rotator.spin",
		Phase:  sdk.PhaseUpdate,
		System: rotatorSystem{},
	})
}

func newRotatorProject(t *testing.T) *project.Project {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Rotator", false)
	if err != nil {
		t.Fatal(err)
	}
	scenePath := filepath.Join(p.ScenesDir(), "main.aescene")
	if err := os.WriteFile(scenePath, []byte(rotatorScene), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNativeComponentDrivesTheScene(t *testing.T) {
	p := newRotatorProject(t)
	app, err := run.Open(p, run.Config{
		Headless:  true,
		Frames:    120,
		Logger:    log.Nop(),
		ScenePath: filepath.Join(p.ScenesDir(), "main.aescene"),
	}, rotatorModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	player := app.Scene.Find(app.Scene.Root(), "Player")
	if player.IsZero() {
		t.Fatal("player node missing")
	}
	if _, ok := scene.Get[Rotator](app.Scene, player); !ok {
		t.Fatal("rotator component was not loaded")
	}
	before := app.Scene.Local(player).Rotation

	driver := app.Driver()
	for i := 0; i < 120; i++ {
		if !app.Engine.Step(driver) {
			t.Fatalf("engine stopped at frame %d: %v", i, app.Engine.Err())
		}
	}
	after := app.Scene.Local(player).Rotation
	if after <= before {
		t.Fatalf("rotation did not advance: %v -> %v", before, after)
	}
}

func TestSceneLoadsWithoutTheNativeModule(t *testing.T) {
	p := newRotatorProject(t)
	app, err := run.Open(p, run.Config{
		Headless:  true,
		Logger:    log.Nop(),
		ScenePath: filepath.Join(p.ScenesDir(), "main.aescene"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	player := app.Scene.Find(app.Scene.Root(), "Player")
	if player.IsZero() {
		t.Fatal("player node missing")
	}
	if _, ok := scene.Get[render2d.SpriteRenderer](app.Scene, player); ok {
		t.Fatal("unexpected component")
	}

	out := sceneioSave(t, app)
	if !containsString(out, "game.Rotator") || !containsString(out, "speed=1.5") {
		t.Fatalf("unknown component data was lost:\n%s", out)
	}
}

func TestCamerasProduceViews(t *testing.T) {
	p := newRotatorProject(t)
	app, err := run.Open(p, run.Config{
		Headless:  true,
		Logger:    log.Nop(),
		ScenePath: filepath.Join(p.ScenesDir(), "main.aescene"),
	}, rotatorModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	driver := app.Driver()
	for i := 0; i < 5; i++ {
		app.Engine.Step(driver)
	}
	if st := app.Runner.Stats(); st.Views != 1 {
		t.Fatalf("views = %d", st.Views)
	}
}

func TestNoAllocationsPerFrameWithScene(t *testing.T) {
	p := newRotatorProject(t)
	app, err := run.Open(p, run.Config{
		Headless:  true,
		Logger:    log.Nop(),
		ScenePath: filepath.Join(p.ScenesDir(), "main.aescene"),
	}, rotatorModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	driver := app.Driver()
	for i := 0; i < 60; i++ {
		app.Engine.Step(driver)
	}
	allocs := testing.AllocsPerRun(240, func() {
		app.Engine.Step(driver)
	})
	if allocs != 0 {
		t.Fatalf("frame allocates %.1f objects with a scene loaded", allocs)
	}
}

func sceneioSave(t *testing.T, app *run.App) string {
	t.Helper()
	return string(sceneio.Save(app.Scene))
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
