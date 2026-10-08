package schema

import (
	"testing"
	"unsafe"

	"aego/core/mathx"
)

type spatial struct {
	Pos  mathx.Vec3   `aego:"pos"`
	Tint mathx.Vec4   `aego:"tint4"`
	Rot  mathx.Quat   `aego:"rot"`
	Path []mathx.Vec3 `aego:"path"`
}

func TestSpatialKinds(t *testing.T) {
	r := NewRegistry()
	td, err := Register[spatial](r, Name("test.Spatial"), Space(Space3D))
	if err != nil {
		t.Fatal(err)
	}
	if td.Space != Space3D {
		t.Fatalf("space = %v", td.Space)
	}
	for name, want := range map[string]Kind{"pos": KindVec3, "tint4": KindVec4, "rot": KindQuat} {
		f, ok := td.Field(name)
		if !ok || f.Kind != want || !f.Fast {
			t.Fatalf("field %s = %+v", name, f)
		}
	}
	if f, _ := td.Field("path"); f.Kind != KindList || f.Elem != KindVec3 {
		t.Fatalf("path = %+v", f)
	}

	var s spatial
	p := unsafe.Pointer(&s)
	q := mathx.QuatEuler(0.1, 0.2, 0.3)
	set := func(name string, v Value) {
		t.Helper()
		f, _ := td.Field(name)
		if err := f.Set(p, v); err != nil {
			t.Fatal(err)
		}
	}
	set("pos", Vec3(mathx.Vec3{X: 1, Y: 2, Z: 3}))
	set("tint4", Vec4(mathx.Vec4{X: 1, W: 0.5}))
	set("rot", Quat(q))
	set("path", List(Vec3(mathx.Vec3{X: 1}), Vec3(mathx.Vec3{Z: 2})))
	if s.Pos != (mathx.Vec3{X: 1, Y: 2, Z: 3}) || s.Tint.W != 0.5 || s.Rot != q || len(s.Path) != 2 || s.Path[1].Z != 2 {
		t.Fatalf("struct = %+v", s)
	}
	var v Value
	f, _ := td.Field("rot")
	f.Get(p, &v)
	if v.Kind != KindQuat || v.Q != q {
		t.Fatalf("get rot = %+v", v)
	}
	f, _ = td.Field("pos")
	if err := f.Set(p, Vec2(mathx.Vec2{})); err == nil {
		t.Fatal("vec2 accepted by a vec3 field")
	}
}

func TestDefaultSpaceIsAny(t *testing.T) {
	type plain struct{ X float32 }
	r := NewRegistry()
	td, err := Register[plain](r, Name("test.Plain"))
	if err != nil {
		t.Fatal(err)
	}
	if td.Space != SpaceAny || !td.Space.Allows(Space3D) || Space2D.Allows(Space3D) {
		t.Fatal("space rules wrong")
	}
}
