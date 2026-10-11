package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/app/run"
	"aego/core/log"
	"aego/core/mathx"
	"aego/project"
	"aego/scene/sceneio"
	"aego/sdk"
)

const roomScene = `scene version=1 space="3d" {
  node "4a5b6c7d8e9f00112233445566778899" name="Camera" enabled=true layers=1 space="3d" {
    transform3d { position=[0, 2, 6] rotation=[-0.1602, 0, 0, 0.9871] scale=[1, 1, 1] }
    component "engine.Camera3D" version=1 {
      fov=60
      near=0.1
      far=1000
      viewport=[0, 0, 1, 1]
      mask=-1
      order=0
      clear=true
      clearColor=#0a0d12ff
      active=true
    }
  }
  node "5b6c7d8e9f0011223344556677889900" name="Floor" enabled=true layers=1 space="3d" {
    transform3d { position=[0, 0, 0] rotation=[0, 0, 0, 1] scale=[1, 1, 1] }
    component "engine.MeshRenderer" version=1 {
      mesh=asset("ae900000000000000000000000000002")
      color=#2e333dff
    }
  }
  node "6c7d8e9f001122334455667788990011" name="Cube" enabled=true layers=1 space="3d" {
    transform3d { position=[0, 0.5, 0] rotation=[0, 0, 0, 1] scale=[1, 1, 1] }
    component "engine.MeshRenderer" version=1 {
      mesh=asset("ae900000000000000000000000000001")
      color=#e07b39ff
    }
    component "game.Rotator3D" version=1 {
      axis=[0, 1, 0]
      speed=1.2
    }
  }
  node "7d8e9f00112233445566778899001122" name="Ball" enabled=true layers=1 space="3d" {
    transform3d { position=[2, 0.5, 0] rotation=[0, 0, 0, 1] scale=[1, 1, 1] }
    component "engine.MeshRenderer" version=1 {
      mesh=asset("ae900000000000000000000000000003")
      color=#52a7c9ff
    }
  }
}
`

type Rotator3D struct {
	Axis  mathx.Vec3 `aego:"axis"`
	Speed float32    `aego:"speed"`
}

type rotator3DSystem struct{}

func (rotator3DSystem) Run(ctx *sdk.Context) {
	s := sdk.SceneOf(ctx)
	if s == nil {
		return
	}
	dt := float32(ctx.Dt)
	sdk.Query1[Rotator3D](s).Each(func(n sdk.NodeHandle, r *Rotator3D) {
		q := mathx.QuatAxisAngle(r.Axis, r.Speed*dt).Mul(s.Local3D(n).Rotation)
		s.SetRotation3D(n, q.Normalized())
	})
}

type rotator3DModule struct{}

func (rotator3DModule) Info() sdk.ModuleInfo {
	return sdk.ModuleInfo{ID: "game.rotator3d", Version: "0.1.0"}
}

func (rotator3DModule) Register(r *sdk.Registry) error {
	if err := sdk.RegisterComponent[Rotator3D](r, sdk.Name("game.Rotator3D")); err != nil {
		return err
	}
	return sdk.RegisterSystem(r, sdk.SystemDesc{
		ID:     "game.rotator3d.spin",
		Phase:  sdk.PhaseUpdate,
		System: rotator3DSystem{},
	})
}

func openRoom(t *testing.T) *run.App {
	t.Helper()
	p, err := project.Create(t.TempDir(), "Room", false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.ScenesDir(), "room.aescene")
	if err := os.WriteFile(path, []byte(roomScene), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := run.Open(p, run.Config{Headless: true, Logger: log.Nop(), ScenePath: path}, rotator3DModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app
}

func TestRoomRendersAndRotates(t *testing.T) {
	app := openRoom(t)
	cube := app.Scene.Find(app.Scene.Root(), "Cube")
	if cube.IsZero() {
		t.Fatal("cube missing")
	}
	driver := app.Driver()
	for i := 0; i < 120; i++ {
		if !app.Engine.Step(driver) {
			t.Fatalf("engine stopped at frame %d: %v", i, app.Engine.Err())
		}
	}
	q := app.Scene.Local3D(cube).Rotation
	if q.Y <= 0 || mathx.Abs(q.X) > 1e-6 || mathx.Abs(q.Z) > 1e-6 {
		t.Fatalf("cube must rotate about +Y: %v", q)
	}
	st := app.Runner.Stats()
	if st.Views != 1 || st.Meshes != 3 {
		t.Fatalf("views %d meshes %d", st.Views, st.Meshes)
	}
}

func TestRoomRoundTrip(t *testing.T) {
	app := openRoom(t)
	out := string(sceneio.Save(app.Scene))
	for _, want := range []string{`space="3d"`, "transform3d", "engine.Camera3D", "game.Rotator3D", "axis=[0, 1, 0]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("saved scene lacks %q:\n%s", want, out)
		}
	}
}

func TestRoomFrameDoesNotAllocate(t *testing.T) {
	app := openRoom(t)
	driver := app.Driver()
	for i := 0; i < 60; i++ {
		app.Engine.Step(driver)
	}
	if allocs := testing.AllocsPerRun(240, func() { app.Engine.Step(driver) }); allocs != 0 {
		t.Fatalf("3D frame allocates %.1f objects", allocs)
	}
}
