package loader

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/texture"
	"testing"
)

func TestEmissionOwnMappingTypedResolverAndAtomicSurfaceReload(t *testing.T) {
	a, n, e := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	ap, np, ep := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	tex := func(size int, semantic asset.TextureSemantic) []byte {
		return asset.EncodeTexture(&asset.TextureArtifact{Width: size, Height: size, Format: gpu.TexRGBA8, Semantic: semantic, Pixels: make([]byte, size*size*4)})
	}
	fs := newFS(t, map[string][]byte{
		"a.tex": textureBytes(8, 8, mathx.White), "n.tex": tex(16, asset.TextureNormal), "e.tex": tex(32, asset.TextureEmissionMask),
		"a.region": asset.EncodeRegion(&asset.RegionArtifact{Page: ap, Normal: n, Emission: e, X: 1, Y: 1, W: 2, H: 3, PageW: 8, PageH: 8, SourceW: 4, SourceH: 4, TrimX: 1}),
		"n.region": asset.EncodeRegion(&asset.RegionArtifact{Page: np, X: 8, Y: 4, W: 4, H: 4, PageW: 16, PageH: 16}),
		"e.region": asset.EncodeRegion(&asset.RegionArtifact{Page: ep, X: 16, Y: 20, W: 3, H: 2, PageW: 32, PageH: 32, SourceW: 4, SourceH: 4, TrimX: 1, TrimY: 2}),
	})
	entries := []asset.Entry{
		{ID: a, Kind: asset.ArtSpriteRegion, Deps: []ids.AssetID{n, e}, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://a.region"}}},
		{ID: n, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://n.region"}}},
		{ID: e, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://e.region"}}},
		{ID: ap, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://a.tex"}}},
		{ID: np, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://n.tex"}}},
		{ID: ep, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://e.tex"}}},
	}
	index := asset.NewIndex()
	for _, entry := range entries {
		index.Put(entry)
	}
	l, _ := newLoader(t)
	if err := l.Mount(index, fs); err != nil {
		t.Fatal(err)
	}
	_, nr, ok := l.NormalFor(a)
	if !ok {
		t.Fatal("normal not resolved")
	}
	ref, er, ok := l.EmissionFor(a)
	if !ok || ref != e || er.Mode != texture.EmissionMask || er.Region.UVMin != (mathx.Vec2{X: .5, Y: .625}) || er.Region.TrimY != 2 {
		t.Fatal("emission reused albedo mapping", er)
	}
	if allocs := testing.AllocsPerRun(100, func() { l.EmissionFor(a) }); allocs != 0 {
		t.Fatal("warm emission association allocates", allocs)
	}
	parameter, ok := l.MaterialParameterTexture(e)
	if !ok || !parameter.RegionOnly || parameter.EmissionMode != texture.EmissionMask || parameter.NormalMap {
		t.Fatal("typed material emission binding lost")
	}
	generation := l.Generation()
	if err := fs.WriteFile("cache://e.tex", tex(32, asset.TextureColor)); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap(entries[5:], fs); err == nil || l.Generation() != generation {
		t.Fatal("incompatible emission replaced live surface")
	}
	if _, kept, ok := l.EmissionFor(a); !ok || kept != er {
		t.Fatal("last-good emission discarded")
	}
	if err := fs.WriteFile("cache://e.tex", tex(32, asset.TextureEmissionRGB)); err != nil {
		t.Fatal(err)
	}
	l.RejectSource(n)
	if err := l.Swap(entries[5:], fs); err == nil {
		t.Fatal("same-size sibling emission update published beside failed normal source")
	}
	if _, kept, ok := l.NormalFor(a); !ok || kept != nr {
		t.Fatal("sibling failed candidate replaced normal")
	}
	if err := l.Swap(entries, fs); err != nil {
		t.Fatal("coherent repaired surface rejected", err)
	}
	if _, next, ok := l.EmissionFor(a); !ok || next.Mode != texture.EmissionRGB || next.Region.Texture == er.Region.Texture {
		t.Fatal("repaired generation not published")
	}
	l.Drop(e)
	if ref, _, ok := l.EmissionFor(a); ok || ref != e {
		t.Fatal("unresolved emission reference was erased")
	}
}
