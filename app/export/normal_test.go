package export

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/asset"
	atlasimp "aego/asset/importers/atlas"
	matimp "aego/asset/importers/material"
	"aego/asset/importers/scenefile"
	teximp "aego/asset/importers/texture"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/gpu/null"
	"aego/project"
	"aego/render/texture"
)

func normalExportProject(t *testing.T, atlas, frames bool) (*project.Project, *pipeline.Pipeline, ids.AssetID, ids.AssetID, ids.AssetID) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "NormalDependencies", false)
	if err != nil {
		t.Fatal(err)
	}
	writeMaterialExportSource(t, p.Root, "scenes/main.aescene", "scene version=1 {}")
	materialDependencyPNG(t, p, "assets/hero.png", color.NRGBA{R: 200, G: 160, B: 100, A: 255})
	materialDependencyPNG(t, p, "assets/hero_n.png", color.NRGBA{R: 128, G: 128, B: 255, A: 255})
	reg := pipeline.NewImporterRegistry()
	for _, imp := range []pipeline.Importer{atlasimp.Wrap(teximp.New()), matimp.New(), scenefile.NewScene()} {
		if err := reg.Register(imp); err != nil {
			t.Fatal(err)
		}
	}
	pl, err := pipeline.Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	p.MainScene, _ = pl.Lookup("scenes/main.aescene")
	albedo, _ := pl.Lookup("assets/hero.png")
	normal, _ := pl.Lookup("assets/hero_n.png")
	if atlas {
		for _, id := range []ids.AssetID{albedo, normal} {
			meta, _ := pl.MetaOf(id)
			meta.Settings["atlas"] = aetext.String("surfaces")
			if err := pl.SetSettings(id, meta.Settings); err != nil {
				t.Fatal(err)
			}
		}
	}
	if frames {
		for _, id := range []ids.AssetID{albedo, normal} {
			if err := pl.SetSubs(id, []pipeline.SubAsset{{Name: "idle", ID: ids.NewAssetID(), Rect: [4]int{0, 0, 1, 2}}, {Name: "run", ID: ids.NewAssetID(), Rect: [4]int{1, 0, 1, 2}}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("surface import: %v %+v", err, batch)
	}
	if frames {
		albedo, _ = pl.Lookup("assets/hero.png#idle")
		normal, _ = pl.Lookup("assets/hero_n.png#idle")
	}
	writeMaterialExportSource(t, p.Root, "assets/lit.aemat", fmt.Sprintf("material version=1 shader=asset(%q) { params { NormalMap=asset(%q) } }", asset.BuiltinShaderLit.String(), normal.String()))
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("Lit import: %v %+v", err, batch)
	}
	mat, _ := pl.Lookup("assets/lit.aemat")
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("valid normal fixture cannot export: %+v", ds)
	}
	return p, pl, albedo, normal, mat
}

func normalExportLoader(t *testing.T, pl *pipeline.Pipeline) *loader.Loader {
	t.Helper()
	d := null.New()
	l := loader.New(d, texture.NewUploader(d), log.Nop())
	if err := l.Mount(pl.Index(), pl.Artifacts()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	return l
}

func TestNormalExportAcceptsIndependentAtlasRegionAndRejectsStalePage(t *testing.T) {
	p, pl, _, normal, mat := normalExportProject(t, true, false)
	old := Take(pl)
	r := &resolver{s: old}
	if info, ok := r.textureInfo(normal); !ok || !info.region || info.semantic != asset.TextureNormal {
		t.Fatal("fixture has no independently packed normal region")
	}
	if _, ok := r.Material(mat); !ok {
		t.Fatal("scene validation cannot resolve a valid normal-region material")
	}
	materialDependencyPNG(t, p, "assets/hero_n.png", color.NRGBA{R: 180, G: 128, B: 220, A: 255})
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("normal page reimport: %v %+v", err, batch)
	}
	if ds := Check(p, old, "release", false); !hasExportError(ds, CodeMaterial, "changed after the export snapshot") {
		t.Fatalf("unchanged normal region authorized stale page pixels: %+v", ds)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("current independently packed normal remains blocked: %+v", ds)
	}
}

func TestNormalExportBlocksExplicitMissingReferenceWithoutDeletingIt(t *testing.T) {
	p, pl, _, _, mat := normalExportProject(t, false, false)
	missing := ids.NewAssetID()
	writeMaterialExportSource(t, p.Root, "assets/lit.aemat", fmt.Sprintf("material version=1 shader=asset(%q) { params { NormalMap=asset(%q) } }", asset.BuiltinShaderLit.String(), missing.String()))
	if batch, err := pl.Import(mat); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("unresolved normal should remain editable: %v %+v", err, batch)
	}
	l := normalExportLoader(t, pl)
	m, ok := l.Material(mat)
	if !ok {
		t.Fatal("explicit missing normal should use runtime flat fallback")
	}
	if _, explicit, resolved := m.Normal(); !explicit || resolved {
		t.Fatal("runtime lost explicit unresolved state")
	}
	for _, profile := range []string{"development", "release"} {
		if ds := Check(p, Take(pl), profile, false); !hasExportError(ds, CodeMaterial, missing.String()) {
			t.Fatalf("%s exported an unresolved explicit normal: %+v", profile, ds)
		}
	}
	e, _ := pl.Entry(mat)
	a, _ := e.Artifact(asset.ArtMaterial)
	data, _ := pl.Artifacts().ReadFile(a.Path)
	doc, err := asset.DecodeMaterial(data, e.Source)
	if err != nil || doc.Textures["NormalMap"] != missing || !slices.Contains(e.Deps, missing) {
		t.Fatal("strict export erased the saved normal reference")
	}
}

func TestNormalExportBlocksMissingAnimationPairAndKeepsFrameID(t *testing.T) {
	p, pl, albedo, normal, _ := normalExportProject(t, false, true)
	sheet, _ := pl.Lookup("assets/hero_n.png")
	meta, _ := pl.MetaOf(sheet)
	if err := pl.SetSubs(sheet, meta.Subs[1:]); err != nil {
		t.Fatal(err)
	}
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("removed frame should retain the association: %v %+v", err, batch)
	}
	e, _ := pl.Entry(albedo)
	a, _ := e.Artifact(asset.ArtSpriteRegion)
	data, _ := pl.Artifacts().ReadFile(a.Path)
	region, err := asset.DecodeRegion(data)
	if err != nil || region.Normal != normal || !slices.Contains(e.Deps, normal) {
		t.Fatal("removed normal frame was silently deleted from the prepared pairing")
	}
	l := normalExportLoader(t, pl)
	if id, _, ok := l.NormalFor(albedo); ok || id != normal {
		t.Fatal("missing normal frame did not retain its ID with flat fallback")
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "paired normal reference is unresolved") {
		t.Fatalf("strict export accepted a missing normal animation frame: %+v", ds)
	}
}

func TestNormalExportRejectsBrokenSourceWhileRuntimeKeepsLastGood(t *testing.T) {
	p, pl, albedo, normal, mat := normalExportProject(t, true, false)
	l := normalExportLoader(t, pl)
	m, ok := l.Material(mat)
	if !ok {
		t.Fatal("valid Lit material failed loading")
	}
	before, explicit, resolved := m.Normal()
	if !explicit || !resolved {
		t.Fatal("normal not loaded")
	}
	if err := os.WriteFile(filepath.Join(p.Root, "assets/hero_n.png"), []byte("broken current normal PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "current") {
		t.Fatalf("broken normal exported before the watcher: %+v", ds)
	}
	if batch, err := pl.Import(normal); err != nil || batch.Failed[normal] == nil {
		t.Fatalf("broken normal did not fail import: %v %+v", err, batch)
	}
	if got, explicit, resolved := m.Normal(); !explicit || !resolved || got != before {
		t.Fatal("strict export or failed import replaced the last-good normal")
	}
	if id, got, ok := l.NormalFor(albedo); !ok || id != normal || got != before {
		t.Fatal("failed normal import broke the loaded sprite pair")
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeImport, "cannot decode") {
		t.Fatalf("broken current normal was exported from last-good pixels: %+v", ds)
	}
}
