package inspector

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/schema"
)

type alphaComp struct {
	V float32 `aego:"v"`
}

type betaComp struct {
	V float32 `aego:"v"`
}

type rowsComp struct {
	Layer int32       `aego:"layer"`
	Order int32       `aego:"order"`
	Tint  mathx.Color `aego:"tint"`
	FlipX bool        `aego:"flipX"`
	FlipY bool        `aego:"flipY"`
	Speed float32     `aego:"speed"`
	Alone bool        `aego:"alone"`
	Name  string      `aego:"name"`
}

func testTypes(t *testing.T) (*scene.Types, *Registry) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[alphaComp](types, schema.Name("test.Alpha")); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[betaComp](types, schema.Name("test.Beta")); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[rowsComp](types, schema.Name("test.Rows")); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := RegisterBuiltin(r, types.Schema()); err != nil {
		t.Fatal(err)
	}
	if err := RegisterMeta[betaComp](r, types.Schema(), Meta{Category: "Physics"}); err != nil {
		t.Fatal(err)
	}
	return types, r
}

func idOf(t *testing.T, reg *schema.Registry, name string) schema.TypeID {
	t.Helper()
	td, ok := reg.ByName(name)
	if !ok {
		t.Fatalf("%s not registered", name)
	}
	return td.ID
}

func TestGroupComponentsOrder(t *testing.T) {
	types, r := testTypes(t)
	reg := types.Schema()
	in := []schema.TypeID{
		idOf(t, reg, "test.Alpha"),
		idOf(t, reg, "engine.SpriteRenderer"),
		idOf(t, reg, "test.Beta"),
		idOf(t, reg, "engine.Camera"),
	}
	got := groupComponents(r, reg, in)
	want := []string{"Rendering", "Camera", "Physics", "test"}
	if len(got) != len(want) {
		t.Fatalf("sections = %+v", got)
	}
	for i, w := range want {
		if got[i].Category != w || len(got[i].Types) != 1 {
			t.Fatalf("section %d = %+v, want %s", i, got[i], w)
		}
	}
}

func TestDefaultMeta(t *testing.T) {
	types, r := testTypes(t)
	td, _ := types.Schema().ByName("test.Alpha")
	m := r.Meta(td)
	if m.DisplayName != "Alpha" || m.Category != "test" || m.Icon != "cat.test" {
		t.Fatalf("meta = %+v", m)
	}
	sr, _ := types.Schema().ByName("engine.SpriteRenderer")
	if r.Meta(sr).DisplayName != "Sprite Renderer" {
		t.Fatal("builtin display name missing")
	}
}

func TestFieldPairs(t *testing.T) {
	types, _ := testTypes(t)
	td, _ := types.Schema().ByName("test.Rows")
	got := plan(td)
	want := []fieldRow{{0, 1}, {2, -1}, {3, 4}, {5, -1}, {6, -1}, {7, -1}}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
