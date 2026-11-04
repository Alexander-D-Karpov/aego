package texture_test

import (
	"bytes"
	"image/color"
	"os"
	"slices"
	"testing"

	"aego/asset"
	"aego/asset/importers/texture"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/gpu"
)

func TestEmissionRolesPreserveStraightRGBAndPackLinearMasks(t *testing.T) {
	imp := texture.New()
	source := pngPixels(t, 2, 2, func(x, y int) color.NRGBA { return color.NRGBA{R: 201, G: 123, B: 45, A: 64} })
	for _, tc := range []struct {
		role, space, channel string
		sem                  asset.TextureSemantic
		format               gpu.TextureFormat
		pixel                [4]byte
	}{
		{"emission-rgb", "srgb", "r", asset.TextureEmissionRGB, gpu.TexSRGBA8, [4]byte{201, 123, 45, 255}},
		{"emission-rgb", "linear", "r", asset.TextureEmissionRGB, gpu.TexRGBA8, [4]byte{201, 123, 45, 255}},
		{"emission-mask", "linear", "g", asset.TextureEmissionMask, gpu.TexRGBA8, [4]byte{123, 123, 123, 255}},
		{"cookie-mask", "linear", "a", asset.TextureCookieMask, gpu.TexRGBA8, [4]byte{64, 64, 64, 255}},
	} {
		t.Run(tc.role+tc.space, func(t *testing.T) {
			s := imp.Defaults()
			s["role"], s["colorspace"], s["mask_channel"] = aetext.Ident(tc.role), aetext.Ident(tc.space), aetext.Ident(tc.channel)
			res, err := imp.Import(&pipeline.ImportContext{Path: "image.png", Source: source, Meta: &pipeline.Meta{Settings: s}})
			if err != nil {
				t.Fatal(err)
			}
			a, err := asset.DecodeTexture(res.Artifacts[0].Data)
			if err != nil {
				t.Fatal(err)
			}
			if a.Semantic != tc.sem || a.Format != tc.format || !slices.Equal(a.Pixels[:4], tc.pixel[:]) {
				t.Fatalf("pixels/color/alpha changed: %+v first=%v", a, a.Pixels[:4])
			}
		})
	}
}
func TestEmissionFrameBindingsSurviveNameOrderAndIndependentSourcePosition(t *testing.T) {
	pl, albedo, emission := pairProject(t, false)
	aFrames, eFrames := cells(2), cells(2)
	// Separate sheets may place matching logical frames at different positions.
	eFrames[0].Rect, eFrames[1].Rect = eFrames[1].Rect, eFrames[0].Rect
	if err := pl.SetSubs(albedo, aFrames); err != nil {
		t.Fatal(err)
	}
	if err := pl.SetSubs(emission, eFrames); err != nil {
		t.Fatal(err)
	}
	a, _ := pl.MetaOf(albedo)
	a.Settings["normal"] = aetext.AssetRef(ids.AssetID{}.String())
	a.Settings["emission"] = aetext.AssetRef(emission.String())
	if err := pl.SetSettings(albedo, a.Settings); err != nil {
		t.Fatal(err)
	}
	e, _ := pl.MetaOf(emission)
	e.Settings["role"] = aetext.Ident("emission-rgb")
	if err := pl.SetSettings(emission, e.Settings); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("import %v %+v", err, batch.Failed)
	}
	for i, f := range aFrames {
		r := regionFromPipeline(t, pl, f.ID)
		if r.Emission != eFrames[i].ID || !r.Normal.IsZero() {
			t.Fatal("frame link mixed channels or position", r)
		}
	}
	if err := pl.Move(emission, "assets/glow_renamed.png"); err != nil {
		t.Fatal(err)
	}
	eFrames[0].Name = "new_name"
	slices.Reverse(eFrames)
	if err := pl.SetSubs(emission, eFrames); err != nil {
		t.Fatal(err)
	}
	batch, err = pl.ImportAll()
	if err != nil || len(batch.Failed) != 0 {
		t.Fatalf("rename %v %+v", err, batch.Failed)
	}
	if regionFromPipeline(t, pl, aFrames[0].ID).Emission != eFrames[1].ID {
		t.Fatal("rename/reorder broke emission frame identity")
	}
	for _, f := range aFrames {
		if err := pl.VerifyCurrent(f.ID); err != nil {
			t.Fatal(err)
		}
	}
}
func TestMetadataWorkingCandidateUsesRealAtlasWithoutMutatingSavedGeneration(t *testing.T) {
	pl, albedo, height := pairProject(t, true)
	for _, id := range []ids.AssetID{albedo, height} {
		m, _ := pl.MetaOf(id)
		m.Settings["atlas"] = aetext.String("surfaces")
		if id == height {
			m.Settings["role"], m.Settings["generate_normal"], m.Settings["height_strength"] = aetext.Ident("height"), aetext.Bool(true), aetext.Float32(1)
		}
		if err := pl.SetSettings(id, m.Settings); err != nil {
			t.Fatal(err)
		}
	}
	source, _ := pl.PathOf(height)
	gradient := pngPixels(t, 32, 16, func(x, y int) color.NRGBA { return color.NRGBA{R: uint8(x * 8), A: 255} })
	if err := os.WriteFile(pl.Abs(source), gradient, 0644); err != nil {
		t.Fatal(err)
	}
	first, err := pl.ImportAll()
	if err != nil || len(first.Failed) > 0 {
		t.Fatalf("initial %v %v", err, first.Failed)
	}
	oldMeta, err := os.ReadFile(pl.Abs(source) + pipeline.MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	changed, _ := pl.MetaOf(height)
	changed.Settings["height_strength"] = aetext.Float32(5)
	if err := pl.SetSettings(height, changed.Settings); err != nil {
		t.Fatal(err)
	}
	saved, err := pl.ImportAll()
	if err != nil || len(saved.Failed) > 0 {
		t.Fatalf("save %v %v", err, saved.Failed)
	}
	savedMeta, _ := os.ReadFile(pl.Abs(source) + pipeline.MetaSuffix)
	region := regionFromPipeline(t, pl, height)
	entry, _ := pl.Entry(region.Page)
	page, _ := entry.Artifact(asset.ArtAtlasPage)
	savedPixels, _ := pl.Artifacts().ReadFile(page.Path)
	candidate, files, err := pl.PreviewMetadata(height, oldMeta, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidatePixels, err := files.ReadFile(page.Path)
	if err != nil || bytes.Equal(candidatePixels, savedPixels) {
		t.Fatal("candidate did not regenerate height normal pixels", err)
	}
	actualMeta, _ := os.ReadFile(pl.Abs(source) + pipeline.MetaSuffix)
	actualPixels, _ := pl.Artifacts().ReadFile(page.Path)
	if !bytes.Equal(actualMeta, savedMeta) || !bytes.Equal(actualPixels, savedPixels) {
		t.Fatal("working candidate changed disk sidecar or cache")
	}
	after, _ := pl.Entry(region.Page)
	a, _ := after.Artifact(asset.ArtAtlasPage)
	if a.Hash != page.Hash {
		t.Fatal("candidate changed persistent index")
	}
	if err := pl.VerifyCurrent(height); err != nil {
		t.Fatal("candidate made saved import stale", err)
	}
	hasAlbedo, hasNormal := false, false
	for _, entry := range candidate.Imported {
		hasAlbedo = hasAlbedo || entry.ID == albedo
		hasNormal = hasNormal || entry.ID == height
	}
	if !hasAlbedo || !hasNormal {
		t.Fatal("candidate omitted a paired surface generation")
	}
	if err := files.WriteFile(page.Path, []byte("bad")); err == nil {
		t.Fatal("published candidate VFS is mutable")
	}
}

func TestTextureSourceResolverSeesNewStableFramesInDirtyMetadata(t *testing.T) {
	pl, _, normal := pairProject(t, false)
	meta, _ := pl.MetaOf(normal)
	frame := ids.NewAssetID()
	meta.Subs = []pipeline.SubAsset{{ID: frame, Name: "new_frame", Rect: [4]int{3, 2, 8, 7}}}
	name, _ := pl.PathOf(normal)
	encoded := pipeline.WriteMeta(&meta)
	overlay := func(path string) ([]byte, bool) {
		if path == name+pipeline.MetaSuffix {
			return encoded, true
		}
		return nil, false
	}
	info, ok := pl.TextureSourceInfo(frame, overlay)
	if !ok || info.ID != frame || info.SourceID != normal || info.Width != 8 || info.Height != 7 || info.Rect != meta.Subs[0].Rect || info.Role != asset.RoleNormal {
		t.Fatal("new dirty frame could not resolve by stable ID", info)
	}
	if _, ok := pl.TextureSourceInfo(frame, nil); ok {
		t.Fatal("dirty frame leaked into persistent source index")
	}
}
