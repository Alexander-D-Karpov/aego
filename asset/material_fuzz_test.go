package asset_test

import (
	"bytes"
	"strings"
	"testing"

	"aego/asset"
)

func FuzzMaterialSourceSemanticRoundTrip(f *testing.F) {
	start := "material version=1 shader=asset(\"00000000000000000000000000000001\") { "
	for _, seed := range []string{
		start + "params {} }",
		start + "params { Index=2147483647 Enabled=true Color=#808080ff Noise=asset(\"00000000000000000000000000000002\") } }",
		start + "params { Removed={ list=[1,2,{ nested=true }] } } extension version=7 { payload=\"retain\" } }",
		start + "/* params { Broken=1 } */ params { X=0.5 X=1 } }",
		start + "variants=[A,B] params { X=NaN } }",
		start + "params { Deep=" + strings.Repeat("[", 40) + "1" + strings.Repeat("]", 40) + " } }",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if len(source) > 60<<10 {
			t.Skip()
		}
		d, err := asset.ParseMaterial(source, "fuzz.aemat")
		if err != nil {
			return
		}
		normal := asset.MarshalMaterial(d)
		parsed, err := asset.ParseMaterial(normal, "saved.aemat")
		if err != nil {
			t.Fatalf("accepted source cannot be saved and reopened: %v", err)
		}
		if !bytes.Equal(normal, asset.MarshalMaterial(parsed)) {
			t.Fatal("source normalization loses semantics on a second save")
		}
		decoded, err := asset.DecodeMaterial(asset.EncodeMaterial(d), "runtime.aemt")
		if err != nil {
			t.Fatalf("accepted document cannot pass through its runtime artifact: %v", err)
		}
		if !bytes.Equal(normal, asset.MarshalMaterial(decoded)) {
			t.Fatal("runtime artifact lost source semantics or an unresolved reference")
		}
	})
}
