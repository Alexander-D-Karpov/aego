package schema

import (
	"testing"
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
)

type testAssetRef struct {
	id ids.AssetID
}

func (r *testAssetRef) AssetRefID() ids.AssetID     { return r.id }
func (r *testAssetRef) SetAssetRefID(v ids.AssetID) { r.id = v }
func (r *testAssetRef) AssetKind() string           { return "texture" }

type testNodeRef struct {
	id ids.NodeID
}

func (r *testNodeRef) NodeRefID() ids.NodeID     { return r.id }
func (r *testNodeRef) SetNodeRefID(v ids.NodeID) { r.id = v }

type blendMode uint8

type inner struct {
	A float32 `aego:"a"`
	B bool    `aego:"b"`
}

type comp struct {
	Max     float32      `aego:"max,min=0,max=1000,doc=upper bound"`
	Current float32      `aego:"current"`
	Count   int32        `aego:"count"`
	Big     int64        `aego:"big"`
	Label   string       `aego:"label"`
	Pos     mathx.Vec2   `aego:"pos"`
	Tint    mathx.Color  `aego:"tint"`
	Area    mathx.Rect   `aego:"area"`
	Blend   blendMode    `aego:"blend"`
	Sprite  testAssetRef `aego:"sprite"`
	Target  testNodeRef  `aego:"target"`
	Nested  inner        `aego:"nested"`
	Tags    []string     `aego:"tags"`
	Parts   []inner      `aego:"parts"`
	Cache   []byte       `aego:"-"`
	Runtime float32      `aego:"runtime,transient"`
	Locked  int32        `aego:"locked,readonly"`
	Auto    float32
	hidden  int
}

func newRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := RegisterEnum[blendMode](r, "test.BlendMode",
		EnumValue{Name: "premultiplied", Value: 0},
		EnumValue{Name: "additive", Value: 1},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterStruct[inner](r, Name("test.Inner")); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRegisterFields(t *testing.T) {
	r := newRegistry(t)
	td, err := Register[comp](r, Name("test.Comp"), Version(2), Doc("component"))
	if err != nil {
		t.Fatal(err)
	}
	if td.ID != HashName("test.Comp") || td.Version != 2 {
		t.Fatalf("descriptor = %+v", td)
	}
	if _, ok := td.Field("cache"); ok {
		t.Fatal("skipped field was registered")
	}
	if _, ok := td.Field("hidden"); ok {
		t.Fatal("unexported field was registered")
	}
	if f, ok := td.Field("auto"); !ok || f.Kind != KindFloat32 {
		t.Fatal("untagged field must default to a lowerCamel name")
	}

	cases := []struct {
		name string
		kind Kind
		fast bool
	}{
		{"max", KindFloat32, true},
		{"count", KindInt32, true},
		{"big", KindInt64, true},
		{"label", KindString, false},
		{"pos", KindVec2, true},
		{"tint", KindColor, true},
		{"area", KindRect, true},
		{"blend", KindEnum, true},
		{"sprite", KindAssetRef, false},
		{"target", KindNodeRef, false},
		{"nested", KindStruct, false},
		{"tags", KindList, false},
		{"parts", KindList, false},
	}
	for _, c := range cases {
		f, ok := td.Field(c.name)
		if !ok {
			t.Fatalf("field %s missing", c.name)
		}
		if f.Kind != c.kind {
			t.Fatalf("field %s kind = %s, want %s", c.name, f.Kind, c.kind)
		}
		if f.Fast != c.fast {
			t.Fatalf("field %s fast = %v, want %v", c.name, f.Fast, c.fast)
		}
	}
	if f, _ := td.Field("max"); f.Min != 0 || f.Max != 1000 || f.Doc != "upper bound" {
		t.Fatalf("tag options lost: %+v", f)
	}
	if f, _ := td.Field("runtime"); f.Flags.Has(FlagSerialize) || !f.Flags.Has(FlagTransient) {
		t.Fatal("transient flag wrong")
	}
	if f, _ := td.Field("locked"); !f.Flags.Has(FlagReadOnly) || !f.Flags.Has(FlagSerialize) {
		t.Fatal("readonly flag wrong")
	}
	if f, _ := td.Field("sprite"); f.AssetKind != "texture" {
		t.Fatalf("asset kind = %q", f.AssetKind)
	}
	if f, _ := td.Field("blend"); len(f.Enum) != 2 || f.EnumName != "test.BlendMode" {
		t.Fatalf("enum metadata lost: %+v", f)
	}
	if f, _ := td.Field("parts"); f.Elem != KindStruct || f.Struct == 0 {
		t.Fatalf("list of structs wrong: %+v", f)
	}
}

func TestAccessorRoundTrip(t *testing.T) {
	r := newRegistry(t)
	td, err := Register[comp](r, Name("test.Comp"))
	if err != nil {
		t.Fatal(err)
	}
	var c comp
	p := unsafe.Pointer(&c)

	assetID := ids.NewAssetID()
	nodeID := ids.NewNodeID()
	writes := []struct {
		field string
		value Value
	}{
		{"max", Float32(42.5)},
		{"count", Int32(7)},
		{"big", Int64(1 << 40)},
		{"label", String("hello")},
		{"pos", Vec2(mathx.Vec2{X: 1, Y: 2})},
		{"tint", Color(mathx.Color{R: 1, A: 1})},
		{"area", Rect(mathx.Rect{X: 1, Y: 2, W: 3, H: 4})},
		{"blend", Enum(1)},
		{"sprite", Asset(assetID)},
		{"target", Node(nodeID)},
		{"tags", List(String("a"), String("b"))},
	}
	for _, w := range writes {
		f, ok := td.Field(w.field)
		if !ok {
			t.Fatalf("field %s missing", w.field)
		}
		if err := f.Set(p, w.value); err != nil {
			t.Fatalf("set %s: %v", w.field, err)
		}
	}
	if c.Max != 42.5 || c.Count != 7 || c.Big != 1<<40 || c.Label != "hello" {
		t.Fatalf("struct = %+v", c)
	}
	if c.Blend != 1 || c.Sprite.id != assetID || c.Target.id != nodeID {
		t.Fatalf("refs or enum not written: %+v", c)
	}
	if len(c.Tags) != 2 || c.Tags[1] != "b" {
		t.Fatalf("list not written: %v", c.Tags)
	}

	var v Value
	f, _ := td.Field("max")
	f.Get(p, &v)
	if v.Kind != KindFloat32 || v.AsFloat() != 42.5 {
		t.Fatalf("get max = %+v", v)
	}
	f, _ = td.Field("sprite")
	f.Get(p, &v)
	if v.Asset != assetID {
		t.Fatal("get asset ref lost the id")
	}
	f, _ = td.Field("tags")
	f.Get(p, &v)
	if v.Kind != KindList || len(v.List) != 2 || v.List[0].S != "a" {
		t.Fatalf("get list = %+v", v)
	}
}

func TestNestedStructAccess(t *testing.T) {
	r := newRegistry(t)
	td, err := Register[comp](r, Name("test.Comp"))
	if err != nil {
		t.Fatal(err)
	}
	var c comp
	p := unsafe.Pointer(&c)
	f, _ := td.Field("nested")
	if err := f.Set(p, Value{Kind: KindStruct, Obj: &Object{
		Type:   f.Struct,
		Fields: []Value{Float32(3.5), Bool(true)},
	}}); err != nil {
		t.Fatal(err)
	}
	if c.Nested.A != 3.5 || !c.Nested.B {
		t.Fatalf("nested = %+v", c.Nested)
	}
	var v Value
	f.Get(p, &v)
	if v.Kind != KindStruct || v.Obj == nil || v.Obj.Fields[0].AsFloat() != 3.5 {
		t.Fatalf("nested get = %+v", v)
	}

	lf, _ := td.Field("parts")
	if err := lf.Set(p, List(
		Value{Kind: KindStruct, Obj: &Object{Type: lf.Struct, Fields: []Value{Float32(1), Bool(false)}}},
		Value{Kind: KindStruct, Obj: &Object{Type: lf.Struct, Fields: []Value{Float32(2), Bool(true)}}},
	)); err != nil {
		t.Fatal(err)
	}
	if len(c.Parts) != 2 || c.Parts[1].A != 2 || !c.Parts[1].B {
		t.Fatalf("parts = %+v", c.Parts)
	}
}

func TestSetRejectsWrongKind(t *testing.T) {
	r := newRegistry(t)
	td, _ := Register[comp](r, Name("test.Comp"))
	var c comp
	f, _ := td.Field("label")
	if err := f.Set(unsafe.Pointer(&c), Int32(1)); !errs.Has(err, errs.SchemaBadValue) {
		t.Fatalf("err = %v", err)
	}
}

func TestUnsupportedFieldIsAnError(t *testing.T) {
	type bad struct {
		Ch chan int `aego:"ch"`
	}
	r := NewRegistry()
	if _, err := Register[bad](r, Name("test.Bad")); !errs.Has(err, errs.SchemaBadField) {
		t.Fatalf("err = %v", err)
	}
	type skipped struct {
		Ch chan int `aego:"-"`
		OK int32    `aego:"ok"`
	}
	if _, err := Register[skipped](r, Name("test.Skipped")); err != nil {
		t.Fatal(err)
	}
}

func TestUnregisteredStructField(t *testing.T) {
	type unreg struct{ X float32 }
	type holder struct {
		U unreg `aego:"u"`
	}
	r := NewRegistry()
	if _, err := Register[holder](r, Name("test.Holder")); !errs.Has(err, errs.SchemaUnknownType) {
		t.Fatalf("err = %v", err)
	}
}

func TestDuplicatesAndCollisions(t *testing.T) {
	type a struct{ X float32 }
	type b struct{ X float32 }
	r := NewRegistry()
	if _, err := Register[a](r, Name("test.Same")); err != nil {
		t.Fatal(err)
	}
	if _, err := Register[b](r, Name("test.Same")); !errs.Has(err, errs.SchemaDuplicate) {
		t.Fatalf("duplicate name accepted: %v", err)
	}
	if _, err := Register[a](r, Name("test.Other")); !errs.Has(err, errs.SchemaDuplicate) {
		t.Fatalf("duplicate go type accepted: %v", err)
	}
}

func TestFormerNames(t *testing.T) {
	type spinner struct {
		Speed float32 `aego:"speed"`
	}
	r := NewRegistry()
	td, err := Register[spinner](r, Name("game.Spinner"), FormerName("game.Rotator"),
		FormerField("speed", "rate"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	old, ok := r.ByName("game.Rotator")
	if !ok || old != td {
		t.Fatal("former type name does not resolve")
	}
	if _, ok := r.ByID(HashName("game.Rotator")); !ok {
		t.Fatal("former type id does not resolve")
	}
	if f, ok := td.Field("rate"); !ok || f.Name != "speed" {
		t.Fatal("former field name does not resolve")
	}
}

func TestFormerNameCollision(t *testing.T) {
	type a struct{ X float32 }
	type b struct{ X float32 }
	r := NewRegistry()
	if _, err := Register[a](r, Name("game.A")); err != nil {
		t.Fatal(err)
	}
	if _, err := Register[b](r, Name("game.B"), FormerName("game.A")); err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze(); !errs.Has(err, errs.SchemaDuplicate) {
		t.Fatalf("collision not reported: %v", err)
	}
}

func TestFrozenRegistryRejectsRegistration(t *testing.T) {
	type a struct{ X float32 }
	type b struct{ X float32 }
	r := NewRegistry()
	if _, err := Register[a](r, Name("test.A")); err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatal(err)
	}
	if !r.Frozen() {
		t.Fatal("registry not frozen")
	}
	if _, err := Register[b](r, Name("test.B")); !errs.Has(err, errs.SchemaFrozen) {
		t.Fatalf("err = %v", err)
	}
	if err := RegisterEnum[blendMode](r, "test.Late", EnumValue{}); !errs.Has(err, errs.SchemaFrozen) {
		t.Fatalf("err = %v", err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatal("second Freeze must be a no-op")
	}
}

func TestLowerFirst(t *testing.T) {
	cases := map[string]string{
		"Max":     "max",
		"FlipX":   "flipX",
		"ID":      "id",
		"URLPath": "urlPath",
		"X":       "x",
	}
	for in, want := range cases {
		if got := lowerFirst(in); got != want {
			t.Fatalf("lowerFirst(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestComponentsAndEach(t *testing.T) {
	r := newRegistry(t)
	if _, err := Register[comp](r, Name("test.Comp")); err != nil {
		t.Fatal(err)
	}
	if n := len(r.Components()); n != 1 {
		t.Fatalf("components = %d", n)
	}
	count := 0
	r.Each(func(*TypeDescriptor) { count++ })
	if count != 2 {
		t.Fatalf("each visited %d types", count)
	}
}

type wideRef struct {
	id ids.AssetID
	_  int
}

func (r *wideRef) AssetRefID() ids.AssetID     { return r.id }
func (r *wideRef) SetAssetRefID(v ids.AssetID) { r.id = v }
func (r *wideRef) AssetKind() string           { return "texture" }

func TestDirectRefLayout(t *testing.T) {
	type refs struct {
		Narrow testAssetRef `aego:"narrow"`
		Wide   wideRef      `aego:"wide"`
		Node   testNodeRef  `aego:"node"`
	}
	r := NewRegistry()
	td, err := Register[refs](r, Name("test.Refs"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"narrow": true, "wide": false, "node": true} {
		if f, _ := td.Field(name); f.Direct != want {
			t.Fatalf("%s direct = %v", name, f.Direct)
		}
	}
}

func TestPersistOption(t *testing.T) {
	type comp struct {
		Hp    int32   `aego:"hp"`
		Speed float32 `aego:"speed"`
		Tmp   float32 `aego:"tmp,transient"`
	}
	r := NewRegistry()
	td, err := Register[comp](r, Name("test.Some"), Persist("hp"))
	if err != nil {
		t.Fatal(err)
	}
	hp, _ := td.Field("hp")
	speed, _ := td.Field("speed")
	if !hp.Flags.Has(FlagPersist) || speed.Flags.Has(FlagPersist) {
		t.Fatal("Persist(names) flags wrong")
	}
	type all struct {
		Hp  int32   `aego:"hp"`
		Tmp float32 `aego:"tmp,transient"`
	}
	td, err = Register[all](r, Name("test.All"), Persist())
	if err != nil {
		t.Fatal(err)
	}
	hp, _ = td.Field("hp")
	tmp, _ := td.Field("tmp")
	if !hp.Flags.Has(FlagPersist) || tmp.Flags.Has(FlagPersist) {
		t.Fatal("Persist() must mark serialized fields only")
	}
	type bad struct {
		Hp int32 `aego:"hp"`
	}
	if _, err := Register[bad](r, Name("test.Bad"), Persist("nope")); !errs.Has(err, errs.SchemaBadField) {
		t.Fatalf("unknown persisted field: %v", err)
	}
}
