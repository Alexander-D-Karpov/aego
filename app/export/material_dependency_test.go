package export

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aego/asset"
	atlasimp "aego/asset/importers/atlas"
	matimp "aego/asset/importers/material"
	"aego/asset/importers/scenefile"
	teximp "aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

func materialDependencyPNG(t *testing.T, p *project.Project, name string, c color.NRGBA) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			im.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(name)), out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func materialDependencyProject(t *testing.T, atlas bool) (*project.Project, *pipeline.Pipeline, []ids.AssetID, ids.AssetID) {
	t.Helper()
	p, err := project.Create(t.TempDir(), "MaterialDependencies", false)
	if err != nil {
		t.Fatal(err)
	}
	writeMaterialExportSource(t, p.Root, "scenes/main.aescene", "scene version=1 {}")
	names := []string{"assets/noise.png"}
	if atlas {
		names = append(names, "assets/second.png")
	}
	for _, name := range names {
		materialDependencyPNG(t, p, name, color.NRGBA{R: 50, G: 100, B: 150, A: 200})
	}
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
	members := make([]ids.AssetID, 0, len(names))
	for _, name := range names {
		id, ok := pl.Lookup(name)
		if !ok {
			t.Fatalf("%s not indexed", name)
		}
		members = append(members, id)
		if atlas {
			meta, _ := pl.MetaOf(id)
			meta.Settings["atlas"] = aetext.String("material-export")
			if err := pl.SetSettings(id, meta.Settings); err != nil {
				t.Fatal(err)
			}
		}
	}
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("texture/scene import: %v %+v", err, batch)
	}
	textureID := members[0]
	if atlas {
		textureID = ids.AssetID{}
		pl.Index().Each(func(e *asset.Entry) {
			if e.Kind == asset.ArtAtlasPage {
				textureID = e.ID
			}
		})
		if textureID.IsZero() {
			t.Fatal("atlas did not produce a page")
		}
	}
	writeMaterialExportSource(t, p.Root, "assets/dissolve.aemat", fmt.Sprintf("material version=1 shader=asset(%q) { params { Noise=asset(%q) } }", asset.BuiltinShaderDissolve.String(), textureID.String()))
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	if batch, err := pl.ImportAll(); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("material import: %v %+v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("valid dependency fixture does not export: %+v", ds)
	}
	return p, pl, members, textureID
}

func TestMaterialExportChecksCurrentTextureDependency(t *testing.T) {
	p, pl, members, _ := materialDependencyProject(t, false)
	if err := os.WriteFile(filepath.Join(p.Root, "assets/noise.png"), []byte("broken current PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The cached texture remains usable by the editor before the watcher sees
	// this write. Strict export must inspect the source as well as that cache.
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "current") {
		t.Fatalf("invalid current texture exported before the watcher: %+v", ds)
	}
	if batch, err := pl.Import(members[0]); err != nil || batch.Failed[members[0]] == nil {
		t.Fatalf("broken PNG did not fail import: %v %+v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeImport, "cannot decode") {
		t.Fatalf("failed current texture import exported: %+v", ds)
	}
	materialDependencyPNG(t, p, "assets/noise.png", color.NRGBA{R: 180, G: 50, B: 25, A: 230})
	if batch, err := pl.Import(members[0]); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("fixed PNG did not reimport: %v %+v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("successfully repaired texture remains blocked: %+v", ds)
	}
}

func TestMaterialExportChecksTextureMetaBeforeWatcher(t *testing.T) {
	p, pl, members, _ := materialDependencyProject(t, false)
	meta, _ := pl.MetaOf(members[0])
	meta.Settings["filter"] = aetext.Ident("linear")
	// Write only metadata on disk: the live record still has the old settings.
	metaPath := filepath.Join(p.Root, "assets/noise.png.meta")
	external := pipeline.WriteMeta(&meta)
	if err := os.WriteFile(metaPath, external, 0o644); err != nil {
		t.Fatal(err)
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "settings changed") {
		t.Fatalf("changed import settings exported the old texture: %+v", ds)
	}
	if err := pl.SetSettings(members[0], meta.Settings); err == nil {
		t.Fatal("stale cached settings overwrote the external metadata")
	}
	if got, err := os.ReadFile(metaPath); err != nil || !bytes.Equal(got, external) {
		t.Fatalf("stale settings changed external metadata: %v", err)
	}
	// The watcher reconciles the already-written metadata and reimports it;
	// SetSettings must not be used to overwrite a newer disk snapshot.
	if batch, err := pl.ReloadMetadata(members[0]); err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("reloaded settings did not import: %v %+v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("reimported texture settings remain blocked: %+v", ds)
	}
}

func TestMaterialExportChecksAllAtlasPageMembers(t *testing.T) {
	p, pl, members, page := materialDependencyProject(t, true)
	if page == members[0] || page == members[1] {
		t.Fatal("fixture did not use a generated page reference")
	}
	// The sampler references the complete page, not either SpriteRegion asset.
	// An edit to either member must invalidate this indirect dependency.
	materialDependencyPNG(t, p, "assets/second.png", color.NRGBA{R: 220, G: 40, B: 80, A: 255})
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "assets/second.png") {
		t.Fatalf("a changed atlas member exported the old page: %+v", ds)
	}
	if batch, err := pl.Import(members[1]); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("atlas member reimport: %v %+v", err, batch)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("repacked page remains blocked: %+v", ds)
	}
	if err := os.Remove(filepath.Join(p.Root, "assets/noise.png")); err != nil {
		t.Fatal(err)
	}
	if ds := Check(p, Take(pl), "release", false); !hasExportError(ds, CodeMaterial, "assets/noise.png") {
		t.Fatalf("a missing atlas member exported the old page: %+v", ds)
	}
}

func TestMaterialExportRejectsStaleTextureSnapshotAfterReimport(t *testing.T) {
	p, pl, members, _ := materialDependencyProject(t, false)
	frozen := Take(pl)
	materialDependencyPNG(t, p, "assets/noise.png", color.NRGBA{R: 250, G: 10, B: 0, A: 255})
	if batch, err := pl.Import(members[0]); err != nil || len(batch.Failed) != 0 {
		t.Fatalf("changed texture reimport: %v %+v", err, batch)
	}
	if ds := Check(p, frozen, "release", false); !hasExportError(ds, CodeMaterial, "") {
		t.Fatalf("live current key authorized texture bytes from an older snapshot: %+v", ds)
	}
	if ds := Check(p, Take(pl), "release", false); Blocking(ds) != 0 {
		t.Fatalf("fresh texture snapshot rejected: %+v", ds)
	}
}
