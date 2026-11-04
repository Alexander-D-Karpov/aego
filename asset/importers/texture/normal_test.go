package texture_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"aego/asset"
	"aego/asset/importers/atlas"
	"aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/gpu"
	"aego/project"
)

func pngPixels(t *testing.T, w, h int, pixel func(x, y int) color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, pixel(x, y))
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestNormalAndHeightPixelsAreLinearData(t *testing.T) {
	imp := texture.New()
	source := pngPixels(t, 3, 3, func(x, y int) color.NRGBA {
		return color.NRGBA{R: uint8(32 + x*64), G: uint8(32 + y*64), B: 255, A: 64}
	})
	importWith := func(settings pipeline.Settings) *asset.TextureArtifact {
		t.Helper()
		res, err := imp.Import(&pipeline.ImportContext{Path: "surface.png", Source: source, Meta: &pipeline.Meta{Settings: settings}})
		if err != nil {
			t.Fatal(err)
		}
		d, err := asset.DecodeTexture(res.Artifacts[0].Data)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	s := imp.Defaults()
	s["semantic"] = aetext.Ident("normal")
	s["normal_y"] = aetext.Ident("down")
	s["mips"] = aetext.Bool(true)
	normal := importWith(s)
	if normal.Semantic != asset.TextureNormal || normal.Format != gpu.TexRGBA8 || !normal.Mips || !slices.Equal(normal.Pixels[:4], []byte{32, 223, 255, 64}) {
		t.Fatalf("normal was color-converted/premultiplied or green not flipped: %+v", normal)
	}
	s["semantic"], s["normal_y"] = aetext.Ident("height"), aetext.Ident("up")
	raw := importWith(s)
	if raw.Semantic != asset.TextureHeight || !slices.Equal(raw.Pixels[:4], []byte{32, 32, 255, 64}) {
		t.Fatal("raw height was implicitly converted")
	}
	s["generate_normal"] = aetext.Bool(true)
	s["height_strength"] = aetext.Float32(4)
	generated := importWith(s)
	center := generated.Pixels[16:20]
	if generated.Semantic != asset.TextureNormal || center[0] >= 100 || center[1] != 128 || center[2] < 200 || center[3] != 64 {
		t.Fatalf("X ramp produced wrong normal: %v", center)
	}
	if other := importWith(s); !bytes.Equal(generated.Pixels, other.Pixels) {
		t.Fatal("height-derived normal is not deterministic")
	}
	s["height_channel"] = aetext.Ident("g")
	up := importWith(s).Pixels[16:20]
	s["normal_y"] = aetext.Ident("down")
	down := importWith(s).Pixels[16:20]
	if up[1] <= 160 || down[1] >= 100 || up[0] != 128 || up[2] != down[2] {
		t.Fatalf("height Y convention wrong: up=%v down=%v", up, down)
	}
	s["height_strength"] = aetext.Float32(0)
	flat := importWith(s).Pixels[16:20]
	if !slices.Equal(flat, []byte{128, 128, 255, 64}) {
		t.Fatalf("zero-strength normal is not flat: %v", flat)
	}
}

func TestHeightGenerationClampsToAnimationFrame(t *testing.T) {
	imp := texture.New()
	s := imp.Defaults()
	s["semantic"], s["generate_normal"] = aetext.Ident("height"), aetext.Bool(true)
	source := pngPixels(t, 4, 2, func(x, y int) color.NRGBA {
		if x < 2 {
			return color.NRGBA{A: 255}
		}
		return color.NRGBA{R: 255, A: 255}
	})
	res, err := imp.Import(&pipeline.ImportContext{Path: "height.png", Source: source, Meta: &pipeline.Meta{Settings: s, Subs: []pipeline.SubAsset{
		{ID: ids.NewAssetID(), Name: "first", Rect: [4]int{0, 0, 2, 2}},
		{ID: ids.NewAssetID(), Name: "second", Rect: [4]int{2, 0, 2, 2}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := asset.DecodeTexture(res.Artifacts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(d.Pixels); i += 4 {
		if !slices.Equal(d.Pixels[i:i+4], []byte{128, 128, 255, 255}) {
			t.Fatalf("normal sampled the neighbouring animation frame at %d: %v", i/4, d.Pixels[i:i+4])
		}
	}
}

func pairProject(t *testing.T, grouped bool) (*pipeline.Pipeline, ids.AssetID, ids.AssetID) {
	t.Helper()
	prj, err := project.Create(t.TempDir(), "Normals", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"hero.png", "hero_n.png"} {
		if err := os.WriteFile(filepath.Join(prj.Root, "assets", name), sheet(t, 32, 16), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reg := pipeline.NewImporterRegistry()
	var imp pipeline.Importer = texture.New()
	if grouped {
		imp = atlas.Wrap(imp)
	}
	if err := reg.Register(imp); err != nil {
		t.Fatal(err)
	}
	pl, err := pipeline.Open(prj, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	albedo, _ := pl.Lookup("assets/hero.png")
	normal, _ := pl.Lookup("assets/hero_n.png")
	return pl, albedo, normal
}

func regionFromPipeline(t *testing.T, pl *pipeline.Pipeline, id ids.AssetID) *asset.RegionArtifact {
	t.Helper()
	e, ok := pl.Entry(id)
	if !ok {
		t.Fatal("missing region entry", id)
	}
	a, ok := e.Artifact(asset.ArtSpriteRegion)
	if !ok {
		t.Fatal("missing region artifact", id)
	}
	data, err := pl.Artifacts().ReadFile(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := asset.DecodeRegion(data)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNormalPairPersistsIDsAndFrameMappingThroughRename(t *testing.T) {
	pl, albedo, normal := pairProject(t, false)
	aFrames, nFrames := cells(2), cells(2)
	if err := pl.SetSubs(albedo, aFrames); err != nil {
		t.Fatal(err)
	}
	if err := pl.SetSubs(normal, nFrames); err != nil {
		t.Fatal(err)
	}
	meta, _ := pl.MetaOf(albedo)
	meta.Settings["user_note"] = aetext.String("keep me")
	if err := pl.SetSettings(albedo, meta.Settings); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("import: %v %v", err, batch.Failed)
	}
	meta, _ = pl.MetaOf(albedo)
	if meta.Settings["normal"].Str != normal.String() || meta.Settings.String("user_note", "") != "keep me" {
		t.Fatalf("association not persisted or user metadata lost: %+v", meta.Settings)
	}
	for i := range aFrames {
		if r := regionFromPipeline(t, pl, aFrames[i].ID); r.Normal != nFrames[i].ID {
			t.Fatalf("frame %d did not bind normal: %+v", i, r)
		}
		if err := pl.VerifyCurrent(aFrames[i].ID); err != nil {
			t.Fatal("first import must already be export-current:", err)
		}
	}
	if err := pl.Move(normal, "assets/renamed_surface.png"); err != nil {
		t.Fatal(err)
	}
	nFrames[0].Name = "renamed_frame"
	if err := pl.SetSubs(normal, nFrames); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("reimport after rename: %v %v", err, batch.Failed)
	}
	if r := regionFromPipeline(t, pl, aFrames[0].ID); r.Normal != nFrames[0].ID {
		t.Fatal("renaming normal file or frame broke stable mapping")
	}
	nmeta, _ := pl.MetaOf(normal)
	if nmeta.Settings.String("semantic", "") != "normal" {
		t.Fatal("normal semantic was not persisted before rename")
	}
	if err := pl.SetSubs(normal, nFrames[1:]); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("removed normal frame: %v %v", err, batch.Failed)
	}
	r := regionFromPipeline(t, pl, aFrames[0].ID)
	e, _ := pl.Entry(aFrames[0].ID)
	if r.Normal != nFrames[0].ID || !slices.Contains(e.Deps, nFrames[0].ID) {
		t.Fatal("unresolved normal frame ID/dependency was deleted")
	}
}

func TestNormalPairUsesIndependentDataAtlas(t *testing.T) {
	pl, albedo, normal := pairProject(t, true)
	for _, id := range []ids.AssetID{albedo, normal} {
		m, _ := pl.MetaOf(id)
		m.Settings["atlas"] = aetext.String("characters")
		m.Settings["trim"] = aetext.Bool(true)
		if err := pl.SetSettings(id, m.Settings); err != nil {
			t.Fatal(err)
		}
	}
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("atlas import: %v %v", err, batch.Failed)
	}
	a, n := regionFromPipeline(t, pl, albedo), regionFromPipeline(t, pl, normal)
	if a.Page == n.Page || a.Normal != normal || a.SourceW != n.SourceW || a.SourceH != n.SourceH {
		t.Fatalf("independent normal atlas mapping wrong: albedo=%+v normal=%+v", a, n)
	}
	for _, tc := range []struct {
		id       ids.AssetID
		semantic asset.TextureSemantic
		format   gpu.TextureFormat
	}{{a.Page, asset.TextureColor, gpu.TexSRGBA8}, {n.Page, asset.TextureNormal, gpu.TexRGBA8}} {
		e, _ := pl.Entry(tc.id)
		art, _ := e.Artifact(asset.ArtAtlasPage)
		data, err := pl.Artifacts().ReadFile(art.Path)
		if err != nil {
			t.Fatal(err)
		}
		page, err := asset.DecodeTexture(data)
		if err != nil || page.Semantic != tc.semantic || page.Format != tc.format {
			t.Fatalf("atlas page semantics wrong: %+v %v", page, err)
		}
		if err := pl.VerifyCurrent(tc.id); err != nil {
			t.Fatal("atlas must be export-current:", err)
		}
	}
}

func TestExplicitMissingNormalWinsOverDiscovery(t *testing.T) {
	pl, albedo, normal := pairProject(t, false)
	missing := ids.NewAssetID()
	m, _ := pl.MetaOf(albedo)
	m.Settings["normal"] = aetext.AssetRef(missing.String())
	if err := pl.SetSettings(albedo, m.Settings); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("missing ref import: %v %v", err, batch.Failed)
	}
	r := regionFromPipeline(t, pl, albedo)
	e, _ := pl.Entry(albedo)
	if r.Normal != missing || slices.Contains(e.Deps, normal) || !slices.Contains(e.Deps, missing) {
		t.Fatalf("explicit unresolved reference was overwritten: %+v %+v", r, e.Deps)
	}
	before, err := os.ReadFile(pl.Abs("assets/hero.png.meta"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pl.Preview("assets/hero.png", sheet(t, 32, 16), nil); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(pl.Abs("assets/hero.png.meta"))
	if !bytes.Equal(before, after) {
		t.Fatal("preview modified source metadata")
	}
}

func TestNormalPairReimportChecksCurrentSourceAndCanvas(t *testing.T) {
	pl, albedo, normal := pairProject(t, false)
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("import: %v %v", err, batch.Failed)
	}
	before := regionFromPipeline(t, pl, albedo)
	if err := os.WriteFile(pl.Abs("assets/hero_n.png"), sheet(t, 16, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pl.VerifyCurrent(albedo); err == nil {
		t.Fatal("strict validation accepted stale normal source")
	}
	batch, err = pl.Reimport(albedo, normal)
	if err != nil || batch.Failed[albedo] == nil {
		t.Fatalf("incompatible normal canvas was not diagnosed: %v %v", err, batch.Failed)
	}
	if current := regionFromPipeline(t, pl, albedo); *current != *before {
		t.Fatal("invalid pair replaced last-good region")
	}
	if err := os.WriteFile(pl.Abs("assets/hero_n.png"), sheet(t, 32, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.Reimport(albedo, normal)
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("pair did not recover: %v %v", err, batch.Failed)
	}
	if err := pl.VerifyCurrent(albedo); err != nil {
		t.Fatal(err)
	}
}

func TestSurfaceSettingsRejectInvalidTypesAndNumbers(t *testing.T) {
	imp := texture.New()
	for _, tc := range []struct {
		name  string
		value aetext.Value
	}{
		{"height_strength", aetext.Float64(math.NaN())}, {"height_strength", aetext.Float64(math.Inf(1))},
		{"height_strength", aetext.Float32(-1)}, {"height_strength", aetext.String("strong")},
		{"height_channel", aetext.Ident("red-ish")}, {"normal_y", aetext.Ident("guess")},
		{"generate_normal", aetext.String("true")}, {"semantic", aetext.Int(3)},
	} {
		s := imp.Defaults()
		s["semantic"] = aetext.Ident("height")
		s["generate_normal"] = aetext.Bool(true)
		s[tc.name] = tc.value
		if _, err := imp.Import(&pipeline.ImportContext{Path: "height.png", Source: sheet(t, 2, 2), Meta: &pipeline.Meta{Settings: s}}); err == nil {
			t.Fatalf("invalid %s accepted: %+v", tc.name, tc.value)
		}
	}
}

func TestNormalPreviewDoesNotPersistDiscovery(t *testing.T) {
	pl, albedo, normal := pairProject(t, false)
	before, err := os.ReadFile(pl.Abs("assets/hero.png.meta"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := pl.Preview("assets/hero.png", sheet(t, 32, 16), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := asset.DecodeRegion(res.Artifacts[1].Data)
	if err != nil || r.Normal != normal {
		t.Fatalf("preview did not resolve private pair: %+v %v", r, err)
	}
	after, err := os.ReadFile(pl.Abs("assets/hero.png.meta"))
	if err != nil {
		t.Fatal(err)
	}
	m, _ := pl.MetaOf(albedo)
	if !bytes.Equal(before, after) {
		t.Fatal("preview wrote metadata")
	}
	if _, exists := m.Settings["normal"]; exists {
		t.Fatal("preview mutated in-memory metadata")
	}
}

func TestChangingAtlasSemanticRetiresTheOldPage(t *testing.T) {
	pl, albedo, normal := pairProject(t, true)
	m, _ := pl.MetaOf(normal)
	m.Settings["atlas"] = aetext.String("surface")
	if err := pl.SetSettings(normal, m.Settings); err != nil {
		t.Fatal(err)
	}
	// Disable the other source's association: this test deliberately turns the
	// normal into mask data and checks atlas ownership, not invalid pairing.
	a, _ := pl.MetaOf(albedo)
	a.Settings["normal"] = aetext.AssetRef((ids.AssetID{}).String())
	if err := pl.SetSettings(albedo, a.Settings); err != nil {
		t.Fatal(err)
	}
	b, err := pl.ImportAll()
	if err != nil || len(b.Failed) != 0 {
		t.Fatalf("initial import: %v %v", err, b.Failed)
	}
	old := regionFromPipeline(t, pl, normal).Page
	m, _ = pl.MetaOf(normal)
	m.Settings["semantic"] = aetext.Ident("mask")
	if err := pl.SetSettings(normal, m.Settings); err != nil {
		t.Fatal(err)
	}
	b, err = pl.Reimport(normal)
	if err != nil || len(b.Failed) != 0 {
		t.Fatalf("semantic reimport: %v %v", err, b.Failed)
	}
	current := regionFromPipeline(t, pl, normal).Page
	if current == old || !slices.Contains(b.Removed, old) {
		t.Fatal("semantic change kept the old atlas page resident")
	}
	if _, exists := pl.Entry(old); exists {
		t.Fatal("empty old atlas page remains in export index")
	}
	if err := pl.VerifyCurrent(current); err != nil {
		t.Fatal(err)
	}
}
