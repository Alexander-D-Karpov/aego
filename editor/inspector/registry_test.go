package inspector

import (
	"reflect"
	"testing"

	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/schema"
)

func TestBuiltinDefaults(t *testing.T) {
	types := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(types); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := RegisterBuiltin(r, types.Schema()); err != nil {
		t.Fatal(err)
	}
	td, _ := types.Schema().ByGoType(reflect.TypeFor[components.SpriteRenderer]())
	tint, _ := td.Field("tint")
	for _, fv := range r.Defaults(td.ID) {
		if fv.Field == tint.ID {
			if fv.Value.C.A != 1 || fv.Value.C.R != 1 {
				t.Fatalf("default tint = %+v", fv.Value.C)
			}
			return
		}
	}
	t.Fatal("tint default missing")
}
