package sceneio

import (
	"testing"

	"aego/core/errs"
	"aego/core/mathx"
	"aego/scene"
)

func TestTransform3DRoundTrip(t *testing.T) {
	s := scene.New(newRegistry(t), nil, scene.Options{DefaultSpace: scene.Space3D})
	cube := s.Create3D(s.Root(), "Cube")
	tr := scene.Transform3D{
		Position: mathx.Vec3{X: 1, Y: 2, Z: 3},
		Rotation: mathx.QuatEuler(0.1, 0.2, 0.3),
		Scale:    mathx.Vec3{X: 1, Y: 2, Z: 1},
	}
	s.SetLocal3D(cube, tr)
	scene.Add[Health](s, cube).Max = 5
	label := s.Create2D(cube, "Label")
	s.SetLocal(label, scene.Transform2D{Position: mathx.Vec2{X: 0.5, Y: -1}, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.Flush()

	out := Save(s)
	for _, want := range []string{
		`scene version=1 space="3d" {`,
		`name="Cube" enabled=true layers=1 space="3d" {`,
		"transform3d {",
		"position=[1, 2, 3]",
		`name="Label" enabled=true layers=1 {`,
	} {
		if !contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	back, report, err := Load(out, "room.aescene", newRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
	if back.DefaultSpace() != scene.Space3D {
		t.Fatal("scene default space lost")
	}
	bc := back.Find(back.Root(), "Cube")
	if back.Space(bc) != scene.Space3D || back.Local3D(bc) != tr {
		t.Fatalf("3d node = %v %+v", back.Space(bc), back.Local3D(bc))
	}
	if bl := back.Find(back.Root(), "Cube/Label"); bl.IsZero() || back.Space(bl) != scene.Space2D {
		t.Fatal("2d child under a 3d node lost its space")
	}
	if string(Save(back)) != string(out) {
		t.Fatal("second save differs")
	}
}

func TestThreeDUnderTwoDInFileFails(t *testing.T) {
	src := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="Flat" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    node "b17e5a3c9d2f4e6a8b0c1d2e3f405162" name="Deep" enabled=true layers=1 space="3d" {
      transform3d { position=[0, 0, 0] rotation=[0, 0, 0, 1] scale=[1, 1, 1] }
    }
  }
}
`)
	if _, _, err := Load(src, "bad.aescene", newRegistry(t), nil, opts()); !errs.Has(err, errs.SceneSpaceMismatch) {
		t.Fatalf("err = %v", err)
	}
}
