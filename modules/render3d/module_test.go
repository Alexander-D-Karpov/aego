package render3d_test

import (
	"testing"

	"aego/asset"
	"aego/core/mathx"
	"aego/engine/module"
	"aego/modules/render3d"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
)

func setup(t *testing.T) (*scene.Types, *module.Manifest) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := frame.RegisterTypes(types); err != nil {
		t.Fatal(err)
	}
	man, err := module.Load(types, nil, render3d.New())
	if err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types, man
}

func spawn(t *testing.T, s *scene.Scene, man *module.Manifest, id string) scene.NodeHandle {
	t.Helper()
	for _, d := range man.Templates {
		if d.ID != id {
			continue
		}
		h := s.Create3D(s.Root(), d.Name)
		d.Setup.(func(*scene.Scene, scene.NodeHandle))(s, h)
		s.Flush()
		return h
	}
	t.Fatalf("template %s missing", id)
	return scene.NodeHandle{}
}

func TestCameraTemplateLooksAtOrigin(t *testing.T) {
	types, man := setup(t)
	s := scene.New(types, nil, scene.Options{})
	h := spawn(t, s, man, "render3d.camera")
	cam, ok := scene.Get[render3d.Camera3D](s, h)
	if !ok || cam.FOV != 60 {
		t.Fatal("camera template did not add a default Camera3D")
	}
	l := s.Local3D(h)
	if fwd := l.Rotation.Rotate(mathx.Vec3{Z: -1}); !fwd.NearlyEqual(l.Position.Neg().Normalized(), 1e-5) {
		t.Fatalf("camera forward = %v", fwd)
	}
}

func TestShapeTemplate(t *testing.T) {
	types, man := setup(t)
	s := scene.New(types, nil, scene.Options{})
	h := spawn(t, s, man, "render3d.cube")
	mr, ok := scene.Get[render3d.MeshRenderer](s, h)
	if !ok || mr.Mesh.ID != asset.BuiltinCube || s.Space(h) != scene.Space3D {
		t.Fatal("cube template has no builtin mesh")
	}
}
