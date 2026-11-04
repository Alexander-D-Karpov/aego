package material

import (
	"testing"

	"aego/core/ids"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/shader"
	"aego/render/texture"
)

func TestSurfaceChannelSemanticsInheritanceAndMigration(t *testing.T) {
	dev := null.New()
	defer dev.Close()
	h, err := dev.CreateTexture(gpu.TextureDesc{Width: 8, Height: 8, Format: gpu.TexRGBA8})
	if err != nil {
		t.Fatal(err)
	}
	sh := testShader("surface", nil, []uint32{0})
	sh.Features = shader.FeatureLighting | shader.FeatureAffine | shader.FeatureSurfaceChannels
	sh.Schema = NewSchema([]shader.Param{
		{Name: "NormalMap", Kind: shader.KindTexture, Normal: true, Slot: shader.TexNormal},
		{Name: "EmissionMap", Kind: shader.KindTexture, Emission: true, Slot: shader.TexEmission},
	}, 0, 0)
	m, ok := New("surface", sh, 0, shader.BlendPremultiplied)
	if !ok {
		t.Fatal("material rejected")
	}
	if _, explicit, _ := m.Emission(); explicit {
		t.Fatal("unset map lost inheritance")
	}
	r := texture.Whole(h, 8, 8).Sub(2, 3, 4, 3)
	bad := Tex{Texture: h, Region: r, RegionOnly: true, NormalMap: true}
	if m.SetTexture("EmissionMap", bad) {
		t.Fatal("normal data was accepted as emission")
	}
	for _, mode := range []texture.EmissionMode{texture.EmissionRGB, texture.EmissionMask} {
		tx := Tex{Texture: h, Region: r, RegionOnly: true, EmissionMode: mode}
		if !m.SetTexture("EmissionMap", tx) {
			t.Fatal("prepared emission region rejected")
		}
		if m.SetTexture("NormalMap", tx) {
			t.Fatal("emission data accepted as normal")
		}
		got, explicit, resolved := m.Emission()
		if !explicit || !resolved || got.Region != r || got.Mode != mode {
			t.Fatalf("mapping/role lost: %+v %v %v", got, explicit, resolved)
		}
	}
	copyShader := *sh
	next, err := m.prepare(&copyShader)
	if err != nil {
		t.Fatal(err)
	}
	if got, explicit, resolved := next.Emission(); !explicit || !resolved || got.Region != r || got.Mode != texture.EmissionMask {
		t.Fatal("reload lost explicit emission")
	}
	if !m.SetTexture("EmissionMap", Tex{}) {
		t.Fatal("explicit empty rejected")
	}
	if got, explicit, resolved := m.Emission(); !explicit || !resolved || got.Valid() {
		t.Fatal("explicit empty was confused with inherited")
	}
	m.Reset("EmissionMap")
	if _, explicit, _ := m.Emission(); explicit {
		t.Fatal("reset did not restore inheritance")
	}
	id := ids.AssetID{Hi: 71, Lo: 19}
	if !m.SetUnresolvedTexture("EmissionMap", id) {
		t.Fatal("missing explicit channel could not be retained")
	}
	if _, explicit, resolved := m.Emission(); !explicit || resolved {
		t.Fatal("unresolved map looked like successful inheritance")
	}
	if got := m.values["EmissionMap"].pending.Reference; got != id {
		t.Fatal("missing AssetID was discarded")
	}
}
