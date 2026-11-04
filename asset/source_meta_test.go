package asset

import (
	"aego/core/ids"
	"aego/format/aetext"
	"bytes"
	"testing"
)

func TestSourceMetadataPreservesUnknownDataFrameOrderAndPivot(t *testing.T) {
	source, first, second := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	raw := []byte(`meta version=1 future="keep" { id="` + source.String() + `" importer="texture" importerVersion=5 futureField={enabled=true} settings { role=color unknown=[1, 2, 3] futureblock { token="stay" } } sub "z" id="` + first.String() + `" rect=[0, 0, 2, 2] pivot=[0.25, 0.75] futureAttr=7 { futureFrame="stay" } sub "a" id="` + second.String() + `" rect=[2, 0, 2, 2] {} unknown "block" { a=1 } }`)
	m, err := ParseSourceMeta(raw, "sheet.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	m.Subs[0].Name = "renamed"
	m.Settings["filter"] = aetext.Ident("linear")
	encoded := MarshalSourceMeta(m)
	for _, text := range []string{"future=", "futureField", "futureblock", "futureAttr", "futureFrame", `unknown "block"`} {
		if !bytes.Contains(encoded, []byte(text)) {
			t.Fatalf("lost %s in %s", text, encoded)
		}
	}
	round, err := ParseSourceMeta(encoded, "sheet.png.meta")
	if err != nil {
		t.Fatal(err)
	}
	if round.Subs[0].ID != first || round.Subs[1].ID != second || round.Subs[0].EffectivePivot() != ([2]float32{.25, .75}) || round.Subs[1].EffectivePivot() != ([2]float32{.5, .5}) {
		t.Fatal("frame order, identity or pivot changed")
	}
}
func TestTextureSourceRolesResolveMetadataNamesAndLinearData(t *testing.T) {
	for _, name := range []string{"hero_n.png", "hero_n.png.meta"} {
		role, err := TextureRoleOf(name, ImportSettings{})
		if err != nil || role != RoleNormal {
			t.Fatalf("%s role=%s err=%v", name, role, err)
		}
	}
	if _, err := TextureSRGB(RoleNormal, ImportSettings{"colorspace": aetext.Ident("srgb")}); err == nil {
		t.Fatal("normal accepted sRGB")
	}
	if linear, err := TextureSRGB(RoleEmissionRGB, ImportSettings{"colorspace": aetext.Ident("linear")}); err != nil || linear {
		t.Fatal("linear emission RGB not represented")
	}
}
