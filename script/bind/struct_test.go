package bind

import (
	"reflect"
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/script/abi"
	"aego/script/bytecode"
	"aego/script/vm"
)

type testMode int32

type inner struct {
	A float32
	B int16
}

type allKinds struct {
	B   bool
	I8  int8
	I16 int16
	I32 int32
	I64 int64
	I   int
	U8  uint8
	U16 uint16
	U32 uint32
	F   float32
	D   float64
	V2  mathx.Vec2
	V3  mathx.Vec3
	C   mathx.Color
	R   mathx.Rect
	Q   mathx.Quat
	N   scene.NodeHandle
	T   asset.Ref[asset.Texture]
	E   testMode
	In  inner
}

type hit struct {
	Point    mathx.Vec2
	Distance float32
	Node     scene.NodeHandle
}

type s8 struct{ A, B, C, D, E, F, G, H float32 }

type s40 struct{ A, B, C, D, E s8 }

func kindsRegistry(t *testing.T) (*Registry, *StructDesc) {
	r := New()
	m := r.Module("test/kinds")
	Enum[testMode](m, "Mode", Member("A", testMode(0)), Member("B", testMode(1)))
	Struct[inner](m, "Inner")
	d := Struct[allKinds](m, "All", Doc("All kinds."))
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	return r, d
}

func TestStructRoundTrip(t *testing.T) {
	r, d := kindsRegistry(t)
	if d.Slots != 21 {
		t.Fatalf("slots %d", d.Slots)
	}
	v := allKinds{
		B: true, I8: -3, I16: -300, I32: -70000, I64: -1 << 40, I: 42, U8: 200, U16: 60000, U32: 4000000000,
		F: 1.25, D: 1.5, V2: mathx.Vec2{X: 1, Y: 2}, V3: mathx.Vec3{X: 3, Y: 4, Z: 5},
		C: mathx.Color{R: 0.1, G: 0.2, B: 0.3, A: 0.4}, R: mathx.Rect{X: 1, Y: 2, W: 3, H: 4},
		Q: mathx.Quat{X: 0, Y: 0, Z: 0.6, W: 0.8}, N: scene.NodeHandle{Index: 3, Gen: 4},
		T: asset.NewRef[asset.Texture](ids.AssetID{Hi: 11, Lo: 12}), E: 1, In: inner{A: 7, B: -8},
	}
	c, err := codecOf[allKinds](r)
	if err != nil {
		t.Fatal(err)
	}
	w := make([]vm.Value, c.slots)
	c.put(nil, w, v)
	if got := c.get(nil, w); !reflect.DeepEqual(got, v) {
		t.Fatalf("round trip\n got %+v\nwant %+v", got, v)
	}
	for _, f := range d.Fields {
		if f.Name == "V2" && w[f.Slot].B != 0 {
			t.Fatal("Vec2 leaves garbage in the second word")
		}
		if f.Name == "In" && (f.Type.Kind != abi.Struct || f.Slots != 2) {
			t.Fatalf("nested field %+v", f)
		}
	}
	if r.Info("test/kinds", "All").Doc != "All kinds." {
		t.Fatal("struct doc is lost")
	}
}

func TestStructRejectsBadFields(t *testing.T) {
	type withString struct{ Name string }
	type withHidden struct{ x int }
	cases := []func(*Module){
		func(m *Module) { Struct[withString](m, "S") },
		func(m *Module) { _ = withHidden{x: 1}; Struct[withHidden](m, "H") },
		func(m *Module) { Struct[s8](m, "S8"); Struct[s40](m, "S40") },
	}
	for i, reg := range cases {
		r := New()
		reg(r.Module("test/bad"))
		err := r.Freeze()
		if err == nil {
			t.Fatalf("case %d accepted", i)
		}
		if i == 0 && !strings.Contains(err.Error(), "Name") {
			t.Fatalf("error does not name the field: %v", err)
		}
	}
}

func hitRegistry(t *testing.T) *Registry {
	r := New()
	m := r.Module("test/phys")
	Struct[hit](m, "Hit")
	Func0(m, "Probe", func(*Ctx) hit {
		return hit{Point: mathx.Vec2{X: 1, Y: 2}, Distance: 3, Node: scene.NodeHandle{Index: 1, Gen: 1}}
	})
	Func1(m, "Fill", func(_ *Ctx, out Out[hit]) int64 {
		n := 0
		for i := 0; i < out.Cap(); i++ {
			if out.Set(i, hit{Distance: float32(i)}) {
				n++
			}
		}
		return int64(n)
	})
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOutWritesAtMostCap(t *testing.T) {
	r := New()
	Struct[hit](r.Module("test/out"), "Hit")
	c, err := codecOf[Out[hit]](r)
	if err != nil {
		t.Fatal(err)
	}
	mc := vm.New(vm.Options{})
	ec, _ := codecOf[hit](r)
	for _, n := range []int{0, 3, 10} {
		var w [1]vm.Value
		if n > 0 {
			w[0] = mc.MakeSlice(ec.slots, 0, make([]vm.Value, n*ec.slots))
		}
		o := c.get(mc, w[:])
		written := 0
		for i := 0; i < 5; i++ {
			if o.Set(i, hit{Distance: float32(i + 1)}) {
				written++
			}
		}
		if o.Cap() != n || written != min(5, n) {
			t.Fatalf("cap %d: Cap()=%d wrote %d", n, o.Cap(), written)
		}
		elems, width, _ := mc.SliceData(w[0])
		for i := 0; i < written; i++ {
			if got := ec.get(mc, elems[i*width:]); got.Distance != float32(i+1) {
				t.Fatalf("cap %d elem %d: %+v", n, i, got)
			}
		}
	}
}

func TestStructCallsDoNotAllocate(t *testing.T) {
	r := hitRegistry(t)
	probe, ok := r.Resolve(bytecode.HostImport{Module: "test/phys", Name: "Probe", Sig: abi.Signature{Result: abi.StructOf("test/phys.Hit")}})
	if !ok {
		t.Fatal("Probe does not resolve")
	}
	fill, ok := r.Resolve(bytecode.HostImport{Module: "test/phys", Name: "Fill", Sig: abi.Sig(abi.T(abi.Int), abi.SliceOf(abi.StructOf("test/phys.Hit")))})
	if !ok {
		t.Fatal("Fill does not resolve")
	}
	mc := vm.New(vm.Options{})
	mc.Env = &Ctx{}
	w := make([]vm.Value, 3)
	buf := mc.MakeSlice(3, 0, make([]vm.Value, 12))
	allocs := testing.AllocsPerRun(1000, func() {
		_ = probe(mc, w)
		w[0] = buf
		_ = fill(mc, w)
	})
	if allocs != 0 {
		t.Fatalf("host calls allocate %v times", allocs)
	}
	if w[0].Int() != 4 {
		t.Fatalf("Fill wrote %d", w[0].Int())
	}
}

func TestEventRegistration(t *testing.T) {
	r := New()
	if err := r.Event(abi.EventDef{Name: "OnHit", Params: []abi.TypeRef{abi.T(abi.Int)}}, Doc("hit")); err != nil {
		t.Fatal(err)
	}
	if err := r.Event(abi.EventDef{Name: "OnHit"}); err == nil {
		t.Fatal("duplicate event accepted")
	}
	if err := r.Event(abi.EventDef{Name: "OnText", Params: []abi.TypeRef{abi.T(abi.String)}}); err != nil {
		t.Fatalf("string parameter rejected: %v", err)
	}
	if err := r.Event(abi.EventDef{Name: "OnList", Params: []abi.TypeRef{abi.SliceOf(abi.T(abi.Int))}}); err == nil {
		t.Fatal("heap parameter accepted")
	}
	if sig, ok := r.Surface().Event("OnHit"); !ok || len(sig.Params) != 1 {
		t.Fatalf("surface: %v %v", sig, ok)
	}
}
