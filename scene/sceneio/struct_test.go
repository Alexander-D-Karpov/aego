package sceneio

import (
	"testing"
	"unsafe"

	"aego/core/mathx"
	"aego/scene"
	"aego/schema"
)

type Wheel struct {
	Radius float32    `aego:"radius"`
	Offset mathx.Vec2 `aego:"offset"`
	Locked bool       `aego:"locked"`
}

type Chassis struct {
	Front Wheel   `aego:"front"`
	Rear  Wheel   `aego:"rear"`
	Mass  float32 `aego:"mass"`
	Axles []Wheel `aego:"axles"`
}

func structRegistry(t *testing.T) *scene.Types {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if _, err := schema.RegisterStruct[Wheel](types.Schema(), schema.Name("test.Wheel")); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[Chassis](types, schema.Name("test.Chassis")); err != nil {
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

func TestNestedStructRoundTrip(t *testing.T) {
	reg := structRegistry(t)
	s := scene.New(reg, nil, scene.Options{})
	h := s.Create(s.Root(), "Car")
	c := scene.Add[Chassis](s, h)
	c.Mass = 1200
	c.Front = Wheel{Radius: 0.35, Offset: mathx.Vec2{X: 1.2, Y: -0.4}, Locked: true}
	c.Rear = Wheel{Radius: 0.4, Offset: mathx.Vec2{X: -1.2, Y: -0.4}}
	c.Axles = []Wheel{
		{Radius: 0.1, Offset: mathx.Vec2{X: 1}},
		{Radius: 0.2, Locked: true},
	}
	s.Flush()

	saved := Save(s)
	if !contains(saved, "front={ radius=0.35") {
		t.Fatalf("nested struct not written:\n%s", saved)
	}
	if !contains(saved, "offset=[1.2, -0.4]") {
		t.Fatalf("nested vec2 not written:\n%s", saved)
	}
	if !contains(saved, "locked=true") {
		t.Fatalf("nested bool not written:\n%s", saved)
	}

	back, report, err := Load(saved, "car.aescene", structRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
	car := back.Find(back.Root(), "Car")
	got, ok := scene.Get[Chassis](back, car)
	if !ok {
		t.Fatal("component lost")
	}
	if got.Mass != 1200 {
		t.Fatalf("mass = %v", got.Mass)
	}
	if got.Front != c.Front || got.Rear != c.Rear {
		t.Fatalf("nested structs differ:\nfront %+v vs %+v\nrear %+v vs %+v",
			got.Front, c.Front, got.Rear, c.Rear)
	}
	if len(got.Axles) != 2 || got.Axles[0].Radius != 0.1 || !got.Axles[1].Locked {
		t.Fatalf("list of structs = %+v", got.Axles)
	}
	if string(Save(back)) != string(saved) {
		t.Fatal("second save differs")
	}
}

func TestNestedStructPartialInput(t *testing.T) {
	src := []byte(`scene version=1 {
  node "3f9a0c21d4e84b7d9a1f2b3c4d5e6f70" name="Car" enabled=true layers=1 {
    transform { position=[0, 0] rotation=0 scale=[1, 1] }
    component "test.Chassis" version=1 {
      front={ radius=0.5 }
      mass=900
    }
  }
}
`)
	s, _, err := Load(src, "partial.aescene", structRegistry(t), nil, opts())
	if err != nil {
		t.Fatal(err)
	}
	car := s.Find(s.Root(), "Car")
	c, ok := scene.Get[Chassis](s, car)
	if !ok {
		t.Fatal("component missing")
	}
	if c.Front.Radius != 0.5 {
		t.Fatalf("radius = %v", c.Front.Radius)
	}
	if c.Front.Offset != (mathx.Vec2{}) || c.Front.Locked {
		t.Fatalf("missing nested fields must stay zero: %+v", c.Front)
	}
	if c.Mass != 900 {
		t.Fatalf("mass = %v", c.Mass)
	}
	if len(c.Axles) != 0 {
		t.Fatalf("absent list must stay empty: %+v", c.Axles)
	}
}

func TestPartialNestedDefaultsAndReferenceRemap(t *testing.T) {
	type slot struct {
		Target scene.NodeRef `aego:"target"`
		Gain   float32       `aego:"gain"`
	}
	type links struct {
		Slots []slot `aego:"slots"`
	}
	types := scene.NewTypes(schema.NewRegistry())
	desc, err := schema.RegisterStruct[slot](types.Schema(), schema.Name("test.Slot"))
	if err != nil {
		t.Fatal(err)
	}
	desc.Default = func() unsafe.Pointer { return unsafe.Pointer(&slot{Gain: .75}) }
	if err := scene.RegisterComponent[links](types, schema.Name("test.Links"), schema.CheckedLiterals()); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	src := []byte(`scene version=1 {
 node "10000000000000000000000000000000" name="Owner" {
  component "test.Links" version=1 {slots=[{target=node("20000000000000000000000000000000")},{gain=0.25}]}
 }
 node "20000000000000000000000000000000" name="Target" {}
}`)
	s, rep, err := Load(src, "links.aescene", types, nil, LoadOptions{Strict: true})
	if err != nil || len(rep.Warnings) != 0 || len(rep.Unresolved) != 0 {
		t.Fatalf("load: %v %+v", err, rep)
	}
	v, ok := scene.Get[links](s, s.Find(s.Root(), "Owner"))
	if !ok || len(v.Slots) != 2 || v.Slots[0].Gain != .75 || v.Slots[1].Gain != .25 || !v.Slots[1].Target.IsZero() {
		t.Fatalf("omitted vs explicit fields: %+v", v)
	}
	if h, ok := v.Slots[0].Target.Resolve(s); !ok || h != s.Find(s.Root(), "Target") {
		t.Fatal("reference inside partial record did not resolve to the remapped node")
	}
}
