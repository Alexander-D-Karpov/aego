package components_test

import (
	"strings"
	"testing"

	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/schema"
)

func types(t *testing.T) *scene.Types {
	t.Helper()
	ty := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(ty); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(ty); err != nil {
		t.Fatal(err)
	}
	if err := ty.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return ty
}

const oldCamera = `scene version=1 {
  node "1a2b3c4d5e6f708192a3b4c5d6e7f809" name="Camera" enabled=true layers=1 {
    transform {
      position=[0, 0]
      rotation=0
      scale=[1, 1]
    }
    component "engine.Camera" version=1 {
      zoom=48
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
}
`

func TestOldCameraLoadsAsCamera2D(t *testing.T) {
	s, report, err := sceneio.Load([]byte(oldCamera), "old.aescene", types(t), nil, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unknown) != 0 || len(report.Warnings) != 0 {
		t.Fatalf("report = %+v", report)
	}
	cam, ok := scene.Get[components.Camera2D](s, s.Find(s.Root(), "Camera"))
	if !ok || cam.Zoom != 48 {
		t.Fatal("former camera did not load as Camera2D")
	}
	out := string(sceneio.Save(s))
	if !strings.Contains(out, `component "engine.Camera2D"`) || strings.Contains(out, `"engine.Camera"`) {
		t.Fatalf("save kept the former name:\n%s", out)
	}
}

func TestComponentsFollowNodeSpace(t *testing.T) {
	s := scene.New(types(t), nil, scene.Options{})
	flat := s.Create2D(s.Root(), "Flat")
	deep := s.Create3D(s.Root(), "Deep")
	mr := scene.Store[components.MeshRenderer](s).Type()
	cam2 := scene.Store[components.Camera2D](s).Type()
	if !s.Accepts(deep, mr) || s.Accepts(flat, mr) {
		t.Fatal("MeshRenderer must be 3D only")
	}
	if !s.Accepts(flat, cam2) || s.Accepts(deep, cam2) {
		t.Fatal("Camera2D must be 2D only")
	}
}
