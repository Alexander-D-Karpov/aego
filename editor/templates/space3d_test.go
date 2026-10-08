package templates_test

import (
	"strings"
	"testing"

	"aego/asset"
	"aego/core/mathx"
	"aego/editor/templates"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/schema"
)

func spatialTypes(t *testing.T) *scene.Types {
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

func TestCameraLooksAtOrigin(t *testing.T) {
	s := scene.New(spatialTypes(t), nil, scene.Options{})
	h := templates.BuildCamera3D(s, s.Root())
	s.Flush()
	if _, ok := scene.Get[components.Camera3D](s, h); !ok || s.Space(h) != scene.Space3D {
		t.Fatal("camera template is not a 3D camera")
	}
	l := s.Local3D(h)
	if fwd := l.Rotation.Rotate(mathx.Vec3{Z: -1}); !fwd.NearlyEqual(l.Position.Neg().Normalized(), 1e-5) {
		t.Fatalf("camera forward = %v", fwd)
	}
}

func TestShapeAndParentRule(t *testing.T) {
	s := scene.New(spatialTypes(t), nil, scene.Options{})
	flat := s.Create2D(s.Root(), "Flat")
	s.Flush()
	parent, note := templates.Parent3D(s, flat)
	if parent != s.Root() || note != templates.Moved3DNote {
		t.Fatalf("parent=%v note=%q", parent, note)
	}
	h := templates.BuildShape3D(s, parent, templates.Shapes3D[0])
	mr, ok := scene.Get[components.MeshRenderer](s, h)
	if !ok || mr.Mesh.ID != asset.BuiltinCube {
		t.Fatal("cube template has no builtin mesh")
	}
}

func TestPrefab3D(t *testing.T) {
	types := spatialTypes(t)
	out := string(templates.Prefab3D(types, templates.BuildCamera3D))
	for _, want := range []string{"prefab", `space="3d"`, "transform3d", `component "engine.Camera3D"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
