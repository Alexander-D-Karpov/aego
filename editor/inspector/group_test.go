package inspector

import (
	"testing"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type alphaComp struct {
	V float32 `aego:"v"`
}

type betaComp struct {
	V float32 `aego:"v"`
}

type spriteComp struct {
	Tint mathx.Color `aego:"tint"`
}

type camComp struct {
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

type testModule struct{}

func (testModule) Info() module.Info {
	return module.Info{ID: "test"}
}

func (testModule) Register(r *module.Registry) error {
	if err := module.Component[alphaComp](r, schema.Name("test.Alpha")).Err(); err != nil {
		return err
	}
	if err := module.Component[betaComp](r, schema.Name("test.Beta")).Category("Physics").Err(); err != nil {
		return err
	}
	if err := module.Component[rowsComp](r, schema.Name("test.Rows")).Err(); err != nil {
		return err
	}
	if err := module.Component[spriteComp](r, schema.Name("fx.Sprite")).
		Category("Rendering").
		Display("Sprite Renderer").
		Default(func() spriteComp { return spriteComp{Tint: mathx.Color{R: 1, G: 1, B: 1, A: 1}} }).
		Err(); err != nil {
		return err
	}
	return module.Component[camComp](r, schema.Name("fx.Cam")).Category("Camera").Err()
}

func testTypes(t *testing.T) (*scene.Types, *Registry) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if _, err := module.Load(types, nil, testModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := r.Load(types.Schema()); err != nil {
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
	in := []schema.TypeID{idOf(t, reg, "test.Alpha"), idOf(t, reg, "fx.Sprite"), idOf(t, reg, "test.Beta"), idOf(t, reg, "fx.Cam")}
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
	if m := r.Meta(td); m.DisplayName != "Alpha" || m.Category != "test" || m.Icon != "cat.test" || m.Badge != "test" {
		t.Fatalf("meta = %+v", m)
	}
	sr, _ := types.Schema().ByName("fx.Sprite")
	if m := r.Meta(sr); m.DisplayName != "Sprite Renderer" || m.Badge != "test" {
		t.Fatalf("meta = %+v", m)
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
