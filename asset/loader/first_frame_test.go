package loader

import (
	"reflect"
	"testing"

	"aego/asset"
	"aego/asset/silhouette"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
)

// A source may own both its full sheet and a first-frame region. A frame
// reference and the source sprite must resolve identical UVs and silhouettes,
// while a regular whole-page PNG remains a valid generic texture sampler.
func TestSourceSpriteFirstFrameAndShadow(t *testing.T) {
	root, first, second, png := ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID(), ids.NewAssetID()
	normalPage, normalFrame := ids.NewAssetID(), ids.NewAssetID()
	emissionPage, emissionFrame := ids.NewAssetID(), ids.NewAssetID()
	page := func(coords ...[2]int) []byte {
		pixels := make([]byte, 8*4*4)
		for _, p := range coords {
			pixels[(p[1]*8+p[0])*4+3] = 255
		}
		return asset.EncodeTexture(&asset.TextureArtifact{Width: 8, Height: 4, Format: gpu.TexRGBA8, Pixels: pixels})
	}
	rootRegion := asset.EncodeRegion(&asset.RegionArtifact{Page: root, W: 4, H: 4, PageW: 8, PageH: 4, Normal: normalFrame, Emission: emissionFrame})
	secondRegion := asset.EncodeRegion(&asset.RegionArtifact{Page: root, X: 4, W: 4, H: 4, PageW: 8, PageH: 4})
	files := newFS(t, map[string][]byte{
		"page": page([2]int{0, 0}, [2]int{6, 2}), "root": rootRegion, "first": rootRegion, "second": secondRegion,
		"png":            textureBytes(4, 4, mathx.White),
		"png-region":     asset.EncodeRegion(&asset.RegionArtifact{Page: png, W: 4, H: 4, PageW: 4, PageH: 4}),
		"normal":         asset.EncodeTexture(&asset.TextureArtifact{Width: 8, Height: 4, Format: gpu.TexRGBA8, Semantic: asset.TextureNormal, Pixels: make([]byte, 8*4*4)}),
		"normal-frame":   asset.EncodeRegion(&asset.RegionArtifact{Page: normalPage, W: 4, H: 4, PageW: 8, PageH: 4}),
		"emission":       asset.EncodeTexture(&asset.TextureArtifact{Width: 8, Height: 4, Format: gpu.TexRGBA8, Semantic: asset.TextureEmissionRGB, Pixels: make([]byte, 8*4*4)}),
		"emission-frame": asset.EncodeRegion(&asset.RegionArtifact{Page: emissionPage, W: 4, H: 4, PageW: 8, PageH: 4}),
	})
	rootEntry := asset.Entry{ID: root, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://page"}, {Kind: asset.ArtSpriteRegion, Path: "cache://root"}}}
	idx := asset.NewIndex()
	for _, entry := range []asset.Entry{
		rootEntry,
		{ID: first, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://first"}}},
		{ID: second, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://second"}}},
		{ID: png, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://png"}, {Kind: asset.ArtSpriteRegion, Path: "cache://png-region"}}},
		{ID: normalPage, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://normal"}}},
		{ID: normalFrame, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://normal-frame"}}},
		{ID: emissionPage, Kind: asset.ArtTexture, Artifacts: []asset.Artifact{{Kind: asset.ArtTexture, Path: "cache://emission"}}},
		{ID: emissionFrame, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://emission-frame"}}},
	} {
		idx.Put(entry)
	}
	l, device := newLoader(t)
	if err := l.Mount(idx, files); err != nil {
		t.Fatal(err)
	}
	// Even before any frame/channel has been accessed, a mismatched first-frame
	// mapping must fail the transaction without publishing or leaking a texture.
	badRegion := asset.EncodeRegion(&asset.RegionArtifact{Page: root, W: 4, H: 4, PageW: 9, PageH: 4})
	if err := files.WriteFile("cache://root", badRegion); err != nil {
		t.Fatal(err)
	}
	generation, resources := l.Generation(), device.ResourceStats()
	if err := l.Swap([]asset.Entry{rootEntry}, files); err == nil || l.Generation() != generation || device.ResourceStats() != resources {
		t.Fatal("mismatched source-frame page size published or leaked a GPU resource")
	}
	if err := files.WriteFile("cache://root", rootRegion); err != nil {
		t.Fatal(err)
	}
	check := func(firstWant, secondWant []silhouette.Rect) {
		t.Helper()
		rootUV, okRoot := l.Texture(root)
		firstUV, okFirst := l.Texture(first)
		secondUV, okSecond := l.Texture(second)
		if !okRoot || !okFirst || !okSecond || rootUV != firstUV || rootUV.W != 4 || rootUV.H != 4 || rootUV.UVMax.X != .5 || secondUV.UVMin.X != .5 || secondUV.Texture != rootUV.Texture {
			t.Fatalf("wrong source/animation regions: root=%+v/%v first=%+v/%v second=%+v/%v", rootUV, okRoot, firstUV, okFirst, secondUV, okSecond)
		}
		rootShape, rootOK := l.SpriteShadow(root)
		firstShape, firstOK := l.SpriteShadow(first)
		secondShape, secondOK := l.SpriteShadow(second)
		if !rootOK || !firstOK || !secondOK || rootShape.W != 4 || rootShape.H != 4 || !reflect.DeepEqual(rootShape, firstShape) || !reflect.DeepEqual(firstShape.Parts, firstWant) || !reflect.DeepEqual(secondShape.Parts, secondWant) {
			t.Fatalf("source/frame silhouettes differ: root=%+v/%v first=%+v/%v second=%+v/%v", rootShape, rootOK, firstShape, firstOK, secondShape, secondOK)
		}
		meta, ok := l.TextureMetadata(root)
		if !ok || !meta.Region || meta.Width != 4 || meta.Height != 4 {
			t.Fatalf("incorrect first-frame metadata: %+v/%v", meta, ok)
		}
		if _, ok := l.MaterialTexture(root); ok {
			t.Fatal("generic material sampler accepted a partial sheet")
		}
		if v, ok := l.MaterialParameterTexture(root); !ok || !v.RegionOnly || v.Region != rootUV {
			t.Fatalf("material parameter lost its frame UVs: %+v/%v", v, ok)
		}
		rootNormal, normalRegion, normalOK := l.NormalFor(root)
		firstNormal, firstNormalRegion, firstNormalOK := l.NormalFor(first)
		if !normalOK || !firstNormalOK || rootNormal != normalFrame || firstNormal != rootNormal || normalRegion != firstNormalRegion || normalRegion.W != 4 || normalRegion.H != 4 {
			t.Fatalf("source/first-frame normal mapping diverged: %s %+v/%v vs %s %+v/%v", rootNormal, normalRegion, normalOK, firstNormal, firstNormalRegion, firstNormalOK)
		}
		rootEmission, emissionRegion, emissionOK := l.EmissionFor(root)
		firstEmission, firstEmissionRegion, firstEmissionOK := l.EmissionFor(first)
		if !emissionOK || !firstEmissionOK || rootEmission != emissionFrame || firstEmission != rootEmission || emissionRegion != firstEmissionRegion || emissionRegion.Region.W != 4 || emissionRegion.Region.H != 4 {
			t.Fatalf("source/first-frame emission mapping diverged: %s %+v/%v vs %s %+v/%v", rootEmission, emissionRegion, emissionOK, firstEmission, firstEmissionRegion, firstEmissionOK)
		}
	}
	check([]silhouette.Rect{{X: 0, Y: 0, W: 1, H: 1}}, []silhouette.Rect{{X: 2, Y: 2, W: 1, H: 1}})
	if meta, ok := l.TextureMetadata(png); !ok || meta.Region || meta.Width != 4 || meta.Height != 4 {
		t.Fatalf("whole PNG metadata regressed: %+v/%v", meta, ok)
	}
	if _, ok := l.MaterialTexture(png); !ok {
		t.Fatal("whole PNG cannot be used as a material texture")
	}
	old, _ := l.Texture(root)
	oldGeneration := l.Generation()
	if err := files.WriteFile("cache://page", page([2]int{1, 1}, [2]int{7, 3})); err != nil {
		t.Fatal(err)
	}
	if err := files.WriteFile("cache://root", []byte("malformed region")); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap([]asset.Entry{rootEntry}, files); err == nil || l.Generation() != oldGeneration {
		t.Fatal("bad first-frame candidate replaced the last-good generation")
	}
	if kept, ok := l.Texture(root); !ok || kept != old {
		t.Fatal("bad candidate changed the published root region")
	}
	if err := files.WriteFile("cache://root", rootRegion); err != nil {
		t.Fatal(err)
	}
	if err := l.Swap([]asset.Entry{rootEntry}, files); err != nil {
		t.Fatal(err)
	}
	check([]silhouette.Rect{{X: 1, Y: 1, W: 1, H: 1}}, []silhouette.Rect{{X: 3, Y: 3, W: 1, H: 1}})
	if next, _ := l.Texture(root); next.Texture == old.Texture {
		t.Fatal("reimport kept the obsolete sheet GPU handle")
	}
}
