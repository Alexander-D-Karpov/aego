package export

import (
	"fmt"
	"image/color"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/render/texture"
)

func TestEmissionExportAcceptsTypedAtlasAndBlocksUnresolvedCurrentReferences(t *testing.T) {
	p, pl, albedo, normal, mat := normalExportProject(t, true, false)
	materialDependencyPNG(t, p, "assets/glow.png", color.NRGBA{R: 230, G: 120, B: 40, A: 32})
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	glow, _ := pl.Lookup("assets/glow.png")
	gm, _ := pl.MetaOf(glow)
	gm.Settings["role"], gm.Settings["atlas"] = aetext.String("emission-rgb"), aetext.String("surfaces")
	if err := pl.SetSettings(glow, gm.Settings); err != nil {
		t.Fatal(err)
	}
	am, _ := pl.MetaOf(albedo)
	am.Settings["emission"] = aetext.AssetRef(glow.String())
	if err := pl.SetSettings(albedo, am.Settings); err != nil {
		t.Fatal(err)
	}
	writeMaterialExportSource(t, p.Root, "assets/lit.aemat", fmt.Sprintf(`material version=1 shader=asset(%q) {params {NormalMap=asset(%q) EmissionMap=asset(%q)}}`, asset.BuiltinShaderLit.String(), normal.String(), glow.String()))
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("emission import %v %+v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("valid emission atlas export blocked %+v", ds)
	}
	l := normalExportLoader(t, pl)
	m, ok := l.Material(mat)
	if !ok {
		t.Fatal("typed Lit emission failed")
	}
	mapped, explicit, resolved := m.Emission()
	if !explicit || !resolved || mapped.Mode != texture.EmissionRGB {
		t.Fatal("explicit emission semantic missing")
	}
	missing := ids.NewAssetID()
	writeMaterialExportSource(t, p.Root, "assets/lit.aemat", fmt.Sprintf(`material version=1 shader=asset(%q) {params {EmissionMap=asset(%q)}}`, asset.BuiltinShaderLit.String(), missing.String()))
	if batch, err := pl.Import(mat); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("missing emission should stay editable %v %v", err, batch)
	}
	repaired := normalExportLoader(t, pl)
	unknown, ok := repaired.Material(mat)
	if !ok {
		t.Fatal("missing emission lost uniform fallback")
	}
	if _, explicit, resolved := unknown.Emission(); !explicit || resolved {
		t.Fatal("missing emission ID state discarded")
	}
	for _, profile := range []string{"development", "release"} {
		if ds := Check(p, Take(pl), profile, false); !hasExportError(ds, CodeMaterial, missing.String()) {
			t.Fatalf("%s exported missing emission: %+v", profile, ds)
		}
	}
	// Independent sprite channel references are strict too, even with no explicit
	// material map. Editing keeps the saved ID; export cannot hide missing source.
	writeMaterialExportSource(t, p.Root, "assets/lit.aemat", fmt.Sprintf(`material version=1 shader=asset(%q) {}`, asset.BuiltinShaderLit.String()))
	am.Settings["emission"] = aetext.AssetRef(missing.String())
	if err := pl.SetSettings(albedo, am.Settings); err != nil {
		t.Fatal(err)
	}
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("unresolved frame emission source should retain ID: %v %v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "paired emission reference is unresolved") {
		t.Fatalf("missing paired emission exported %+v", ds)
	}
}
