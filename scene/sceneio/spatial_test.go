package sceneio

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

type Orbit struct {
	Center mathx.Vec3   `aego:"center"`
	Spin   mathx.Quat   `aego:"spin"`
	Tilt   mathx.Quat   `aego:"tilt"`
	Mix    mathx.Vec4   `aego:"mix"`
	Points []mathx.Vec3 `aego:"points"`
}

func orbitTypes(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Orbit](types, schema.Name("test.Orbit")); err != nil {
		t.Fatal(err)
	}
	if err := RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return types
}

func TestSpatialValuesRoundTrip(t *testing.T) {
	s := scene.New(orbitTypes(t), nil, scene.Options{})
	h := s.Create(s.Root(), "Planet")
	o := scene.Add[Orbit](s, h)
	o.Center = mathx.Vec3{X: 1, Y: 2, Z: 3}
	o.Spin = mathx.QuatIdentity()
	o.Tilt = mathx.QuatEuler(0.2, -0.4, 1.1)
	o.Mix = mathx.Vec4{X: 0.5, Y: 0.25, Z: 0, W: 1}
	o.Points = []mathx.Vec3{{X: 1}, {Y: -2, Z: 0.5}}
	s.Flush()

	out := Save(s)
	for _, want := range []string{"center=[1, 2, 3]", "spin=[0, 0, 0, 1]", "mix=[0.5, 0.25, 0, 1]", "points=[[1, 0, 0], [0, -2, 0.5]]"} {
		if !contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	back, report, err := Load(out, "orbit.aescene", orbitTypes(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
	got, ok := scene.Get[Orbit](back, back.Find(back.Root(), "Planet"))
	if !ok {
		t.Fatal("component lost")
	}
	if got.Center != o.Center || got.Spin != o.Spin || got.Tilt != o.Tilt || got.Mix != o.Mix || len(got.Points) != 2 || got.Points[1] != o.Points[1] {
		t.Fatalf("round trip = %+v", got)
	}
	if string(Save(back)) != string(out) {
		t.Fatal("second save differs")
	}
}

func TestMissingQuatDefaultsToIdentity(t *testing.T) {
	src := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="P" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "test.Orbit" version=1 {
      spin=[1, 2]
    }
  }
}
`)
	s, _, err := Load(src, "bad.aescene", orbitTypes(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	o, _ := scene.Get[Orbit](s, s.Find(s.Root(), "P"))
	if o.Spin != mathx.QuatIdentity() {
		t.Fatalf("malformed quat = %+v", o.Spin)
	}
}
