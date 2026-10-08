package bind

import (
	"strings"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/script/abi"
	bc "aego/script/bytecode"
	"aego/script/vm"
)

type testKey uint16

type plainInt int32

func roundTrip[T comparable](t *testing.T, r *Registry, v T, want abi.TypeRef) {
	t.Helper()
	c, err := codecOf[T](r)
	if err != nil {
		t.Fatal(err)
	}
	if !c.ref.Equal(want) || c.slots != want.Slots() {
		t.Fatalf("%T: ref %s slots %d, want %s", v, c.ref, c.slots, want)
	}
	m := vm.New(vm.Options{})
	w := make([]vm.Value, c.slots)
	c.put(m, w, v)
	if got := c.get(m, w); got != v {
		t.Fatalf("%T: %v -> %v", v, v, got)
	}
}

func TestCodecs(t *testing.T) {
	r := New()
	Enum[testKey](r.Module("t"), "Key", Member("A", testKey(4)))
	roundTrip(t, r, true, abi.T(abi.Bool))
	roundTrip(t, r, int64(-7), abi.T(abi.Int))
	roundTrip(t, r, 42, abi.T(abi.Int))
	roundTrip(t, r, float32(1.5), abi.T(abi.Float))
	roundTrip(t, r, 2.25, abi.T(abi.Float))
	roundTrip(t, r, "жук", abi.T(abi.String))
	roundTrip(t, r, mathx.Vec2{X: 1, Y: -2}, abi.T(abi.Vec2))
	roundTrip(t, r, mathx.Vec3{X: 1, Y: 2, Z: 3}, abi.T(abi.Vec3))
	roundTrip(t, r, mathx.Color{R: 0.1, G: 0.2, B: 0.3, A: 1}, abi.T(abi.Color))
	roundTrip(t, r, mathx.QuatEuler(0.1, 0.2, 0.3), abi.T(abi.Quat))
	roundTrip(t, r, mathx.Rect{X: 1, Y: 2, W: 3, H: 4}, abi.T(abi.Rect))
	roundTrip(t, r, scene.Transform2D{Position: mathx.Vec2{X: 1, Y: 2}, Rotation: 0.5, Scale: mathx.Vec2{X: 2, Y: 2}}, abi.T(abi.Transform2D))
	roundTrip(t, r, scene.Transform3D{Position: mathx.Vec3{Z: 1}, Rotation: mathx.QuatIdentity(), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}}, abi.T(abi.Transform3D))
	roundTrip(t, r, scene.NodeHandle{Index: 3, Gen: 2}, abi.T(abi.NodeRef))
	roundTrip(t, r, testKey(4), abi.EnumOf("t.Key"))

	c, err := codecOf[asset.Ref[asset.Texture]](r)
	if err != nil || !c.ref.Equal(abi.AssetOf("texture")) {
		t.Fatalf("texture ref %s %v", c.ref, err)
	}
	id := ids.NewAssetID()
	w := make([]vm.Value, 1)
	c.put(nil, w, asset.NewRef[asset.Texture](id))
	if got := c.get(nil, w); got.ID != id {
		t.Fatal("asset id lost")
	}

	vc, _ := codecOf[mathx.Vec2](r)
	vc.put(nil, w, mathx.Vec2{X: 1, Y: 2})
	if w[0].B != 0 {
		t.Fatal("Vec2 must leave the upper lanes zero")
	}
	if _, err := codecOf[chan int](r); err == nil {
		t.Fatal("chan accepted")
	}
	if _, err := codecOf[plainInt](r); err == nil {
		t.Fatal("unregistered named int accepted")
	}
}

func freeze(t *testing.T, r *Registry) {
	t.Helper()
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T, r *Registry, mod *bc.Module, ctx *Ctx) (*vm.Machine, *vm.Module) {
	t.Helper()
	m := vm.New(vm.Options{})
	m.Env = ctx
	lm, err := m.Load(mod, r.Resolve)
	if err != nil {
		t.Fatal(err)
	}
	return m, lm
}

func TestAdaptersThroughVM(t *testing.T) {
	r := New()
	m := r.Module("t")
	Func2(m, "Add", func(_ *Ctx, a, b int64) int64 { return a + b })
	Func2(m, "Spin", func(_ *Ctx, tr scene.Transform2D, k float32) float32 { return tr.Rotation*k + tr.Position.Y })
	freeze(t, r)
	pos := vm.Vec(0, 3, 0, 0)
	mod := bc.NewModule("test")
	mod.Imports = []bc.HostImport{
		{Module: "t", Name: "Add", Sig: abi.Sig(abi.T(abi.Int), abi.T(abi.Int), abi.T(abi.Int))},
		{Module: "t", Name: "Spin", Sig: abi.Sig(abi.T(abi.Float), abi.T(abi.Transform2D), abi.T(abi.Float))},
	}
	mod.Functions = []bc.Function{
		{Name: "add", Results: 1, Registers: 3, Script: -1, Code: []bc.Instr{
			bc.AsBx(bc.LOADI, 1, 2), bc.AsBx(bc.LOADI, 2, 40), bc.ABx(bc.CALLH, 1, 0), bc.ABC(bc.RET, 1, 1, 0),
		}},
		{Name: "spin", Results: 1, Registers: 5, Script: -1,
			Consts: []bc.Const{bc.RawK(pos.A, pos.B), bc.FloatK(0.5), bc.FloatK(4)},
			Code: []bc.Instr{
				bc.ABx(bc.LOADK, 1, 0), bc.ABx(bc.LOADK, 2, 1), bc.ABC(bc.LOADNIL, 3, 0, 0), bc.ABx(bc.LOADK, 4, 2),
				bc.ABx(bc.CALLH, 1, 1), bc.ABC(bc.RET, 1, 1, 0),
			}},
	}
	mc, lm := load(t, r, mod, &Ctx{})
	out, fl := mc.Call(lm.Funcs[0], nil, nil)
	if fl != nil || out[0].Int() != 42 {
		t.Fatalf("add %v %v", out, fl)
	}
	out, fl = mc.Call(lm.Funcs[1], nil, nil)
	if fl != nil || out[0].Float() != 5 {
		t.Fatalf("spin %v %v", out, fl)
	}
}

func procModule(names ...string) *bc.Module {
	m := bc.NewModule("test")
	for i, n := range names {
		m.Imports = append(m.Imports, bc.HostImport{Module: "t", Name: n, Sig: abi.Sig(abi.T(abi.Void))})
		m.Functions = append(m.Functions, bc.Function{Name: n, Registers: 1, Script: -1, Code: []bc.Instr{
			bc.ABx(bc.CALLH, 0, uint16(i)), bc.ABC(bc.RET, 0, 0, 0),
		}})
	}
	return m
}

func TestFailPanicAndMissingCtx(t *testing.T) {
	r := New()
	m := r.Module("t")
	Proc0(m, "Bad", func(c *Ctx) { c.Fail("boom") })
	Proc0(m, "Crash", func(*Ctx) { panic("bad binding") })
	freeze(t, r)
	ctx := &Ctx{}
	mc, lm := load(t, r, procModule("Bad", "Crash"), ctx)
	if _, fl := mc.Call(lm.Funcs[0], nil, nil); fl == nil || fl.Code != vm.CodeHost || fl.Message != "boom" {
		t.Fatalf("fail %+v", fl)
	}
	if ctx.Failed() {
		t.Fatal("failure leaked into the next call")
	}
	if _, fl := mc.Call(lm.Funcs[1], nil, nil); fl == nil || fl.Code != vm.CodeNativePanic {
		t.Fatalf("panic %+v", fl)
	}
	mc, lm = load(t, r, procModule("Bad"), nil)
	if _, fl := mc.Call(lm.Funcs[0], nil, nil); fl == nil || !strings.Contains(fl.Message, "binding context") {
		t.Fatalf("no ctx %+v", fl)
	}
}

func TestMethodsResolve(t *testing.T) {
	r := New()
	Func1(r.Methods(abi.NodeRef), "Index", func(_ *Ctx, h scene.NodeHandle) int64 { return int64(h.Index) })
	freeze(t, r)
	if sig, ok := r.Surface().Method(abi.NodeRef, "Index"); !ok || len(sig.Params) != 0 || sig.Result.Kind != abi.Int {
		t.Fatalf("surface %v %v", sig, ok)
	}
	imp := bc.HostImport{Module: "#NodeRef", Name: "Index", Sig: abi.Sig(abi.T(abi.Int), abi.T(abi.NodeRef))}
	if _, ok := r.Resolve(imp); !ok {
		t.Fatal("method import not resolved")
	}
	imp.Sig = abi.Sig(abi.T(abi.Float), abi.T(abi.NodeRef))
	if _, ok := r.Resolve(imp); ok {
		t.Fatal("wrong signature resolved")
	}
}

func TestRegistrationErrors(t *testing.T) {
	r := New()
	m := r.Module("t")
	Func1(m, "Chan", func(_ *Ctx, c chan int) int64 { return 0 })
	Proc0(m, "Twice", func(*Ctx) {})
	Proc0(m, "Twice", func(*Ctx) {})
	Func0(r.Methods(abi.NodeRef), "NoRecv", func(*Ctx) int64 { return 0 })
	err := r.Freeze()
	if err == nil {
		t.Fatal("errors were swallowed")
	}
	for _, want := range []string{"t.Chan", "t.Twice", "#NodeRef.NoRecv"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %v", want, err)
		}
	}
	Proc0(m, "Late", func(*Ctx) {})
	if err := r.Freeze(); err == nil || !strings.Contains(err.Error(), "t.Late") {
		t.Fatalf("late registration accepted: %v", err)
	}
}

func TestABIHashIgnoresOrder(t *testing.T) {
	add := func(_ *Ctx, x, y int64) int64 { return x + y }
	neg := func(_ *Ctx, x float32) float32 { return -x }
	a, b, c := New(), New(), New()
	Func2(a.Module("m"), "Add", add)
	Func1(a.Module("n"), "Neg", neg)
	Func1(b.Module("n"), "Neg", neg)
	Func2(b.Module("m"), "Add", add)
	Func2(c.Module("m"), "Add", add)
	freeze(t, a)
	freeze(t, b)
	freeze(t, c)
	if a.ABIHash() != b.ABIHash() {
		t.Fatal("hash depends on registration order")
	}
	if a.ABIHash() == c.ABIHash() {
		t.Fatal("hash ignores a missing function")
	}
}
