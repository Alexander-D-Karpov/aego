package inspector

import "testing"

func TestDefaultsFromDescriptor(t *testing.T) {
	types, r := testTypes(t)
	td, _ := types.Schema().ByName("fx.Sprite")
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
