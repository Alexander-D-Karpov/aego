package loader

import (
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
)

func TestNormalForUsesOwnUVTrimAndSurvivesReload(t *testing.T) {
	albedo, normal, aPage, nPage := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	normalData := asset.EncodeTexture(&asset.TextureArtifact{Width: 64, Height: 64, Format: gpu.TexRGBA8, Semantic: asset.TextureNormal, Pixels: make([]byte, 64*64*4)})
	fs := newFS(t, map[string][]byte{
		"a.aetex": textureBytes(32, 32, mathx.White), "n.aetex": normalData,
		"a.aereg": asset.EncodeRegion(&asset.RegionArtifact{Page: aPage, Normal: normal, X: 2, Y: 2, W: 4, H: 3, PageW: 32, PageH: 32, SourceW: 8, SourceH: 8, TrimX: 2, TrimY: 1}),
		"n.aereg": asset.EncodeRegion(&asset.RegionArtifact{Page: nPage, X: 16, Y: 24, W: 5, H: 4, PageW: 64, PageH: 64, SourceW: 8, SourceH: 8, TrimX: 1, TrimY: 2}),
	})
	idx := asset.NewIndex()
	entries := []asset.Entry{
		{ID: albedo, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://a.aereg"}}},
		{ID: normal, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://n.aereg"}}},
		{ID: aPage, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://a.aetex"}}},
		{ID: nPage, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://n.aetex"}}},
	}
	for _, e := range entries {
		idx.Put(e)
	}
	l, device := newLoader(t)
	if err := l.Mount(idx, fs); err != nil {
		t.Fatal(err)
	}
	resources := device.ResourceStats()
	info, described := l.TextureMetadata(normal)
	if !described || !info.Region || info.Width != 8 || info.Height != 8 || info.Semantic != asset.TextureNormal {
		t.Fatalf("headless metadata lost normal source semantics: %+v", info)
	}
	if device.ResourceStats() != resources || l.Resident() != 0 {
		t.Fatal("metadata validation allocated a GPU texture")
	}
	id, region, ok := l.NormalFor(albedo)
	if !ok || id != normal || region.UVMin != (mathx.Vec2{X: .25, Y: .375}) || region.TrimX != 1 || region.TrimY != 2 || region.SourceW != 8 {
		t.Fatalf("normal reused albedo mapping: id=%s region=%+v ok=%v", id, region, ok)
	}
	if allocs := testing.AllocsPerRun(100, func() { l.NormalFor(albedo) }); allocs != 0 {
		t.Fatalf("warmed normal association allocates: %v", allocs)
	}
	if _, ok := l.MaterialTexture(normal); ok {
		t.Fatal("ordinary noise/data sampler accepted an atlas region")
	}
	tex, ok := l.MaterialParameterTexture(normal)
	if !ok || !tex.RegionOnly || !tex.NormalMap || tex.Region != region {
		t.Fatalf("@normal resolver lost region/data semantic: %+v", tex)
	}
	if err := l.Swap(entries[3:], fs); err != nil {
		t.Fatal(err)
	}
	_, next, ok := l.NormalFor(albedo)
	if !ok || next.Texture == region.Texture || next.UVMin != region.UVMin || next.TrimX != region.TrimX {
		t.Fatal("normal page reload kept stale GPU texture or changed mapping")
	}
	before := l.Generation()
	if err := fs.WriteFile("cache://a.aereg", []byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap(entries[:1], fs); err == nil || l.Generation() != before {
		t.Fatal("malformed region reload must retain last-good association")
	}
	if id, kept, ok := l.NormalFor(albedo); !ok || id != normal || kept != next {
		t.Fatal("broken region reload replaced last-good normal mapping")
	}
	l.Drop(normal)
	if id, _, ok := l.NormalFor(albedo); ok || id != normal {
		t.Fatal("dropping a normal asset deleted its saved reference")
	}
}

func TestStandaloneNormalAssociationAndWrongSemantic(t *testing.T) {
	albedo, normal := ids.NewAssetID(), ids.NewAssetID()
	fs := newFS(t, map[string][]byte{
		"a.aetex": textureBytes(4, 4, mathx.White),
		"a.aereg": asset.EncodeRegion(&asset.RegionArtifact{Page: albedo, Normal: normal, W: 4, H: 4, PageW: 4, PageH: 4}),
		"n.aetex": textureBytes(4, 4, mathx.White),
	})
	idx := asset.NewIndex()
	idx.Put(asset.Entry{ID: albedo, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{
		{Kind: asset.ArtTexture, Path: "cache://a.aetex"}, {Kind: asset.ArtSpriteRegion, Path: "cache://a.aereg"},
	}})
	idx.Put(asset.Entry{ID: normal, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://n.aetex"}}})
	l, _ := newLoader(t)
	if err := l.Mount(idx, fs); err != nil {
		t.Fatal(err)
	}
	if id, _, ok := l.NormalFor(albedo); ok || id != normal {
		t.Fatal("color texture silently accepted as normal data or reference lost")
	}
}

func TestNormalPartialResizeKeepsLastGoodGPUResources(t *testing.T) {
	albedo, normal := ids.NewAssetID(), ids.NewAssetID()
	normalData := func(size int) []byte {
		return asset.EncodeTexture(&asset.TextureArtifact{Width: size, Height: size, Format: gpu.TexRGBA8, Semantic: asset.TextureNormal, Pixels: make([]byte, size*size*4)})
	}
	fs := newFS(t, map[string][]byte{"a.aetex": textureBytes(4, 4, mathx.White), "n.aetex": normalData(4),
		"a.aereg": asset.EncodeRegion(&asset.RegionArtifact{Page: albedo, Normal: normal, W: 4, H: 4, PageW: 4, PageH: 4}),
	})
	aEntry := asset.Entry{ID: albedo, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://a.aetex"}, {Kind: asset.ArtSpriteRegion, Path: "cache://a.aereg"}}}
	nEntry := asset.Entry{ID: normal, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://n.aetex"}}}
	idx := asset.NewIndex()
	idx.Put(aEntry)
	idx.Put(nEntry)
	l, _ := newLoader(t)
	if err := l.Mount(idx, fs); err != nil {
		t.Fatal(err)
	}
	_, before, ok := l.NormalFor(albedo)
	if !ok {
		t.Fatal("initial normal unresolved")
	}
	generation := l.Generation()
	if err := fs.WriteFile("cache://n.aetex", normalData(2)); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap([]asset.Entry{nEntry}, fs); err == nil {
		t.Fatal("partial incompatible normal resize published")
	}
	_, kept, ok := l.NormalFor(albedo)
	if !ok || kept != before || l.Generation() != generation {
		t.Fatal("invalid resize discarded last-good normal resource")
	}
	if err := fs.WriteFile("cache://a.aetex", textureBytes(2, 2, mathx.White)); err != nil {
		t.Fatal(err)
	}
	if err := fs.WriteFile("cache://a.aereg", asset.EncodeRegion(&asset.RegionArtifact{Page: albedo, Normal: normal, W: 2, H: 2, PageW: 2, PageH: 2})); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap([]asset.Entry{aEntry, nEntry}, fs); err != nil {
		t.Fatal("matching resize rejected", err)
	}
	_, next, ok := l.NormalFor(albedo)
	if !ok || next.W != 2 || next.Texture == before.Texture {
		t.Fatal("matching albedo+normal resize did not publish together")
	}
}
