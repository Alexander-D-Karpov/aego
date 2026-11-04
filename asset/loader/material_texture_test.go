package loader

import (
	"fmt"
	"testing"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/render/material"
	"aego/render/shader"
)

func TestPreviewAndRuntimeSamplersRejectAtlasRegions(t *testing.T) {
	page, region := ids.AssetID{Lo: 601}, ids.AssetID{Lo: 602}
	files := newFS(t, map[string][]byte{
		"page.aetex": textureBytes(32, 32, mathx.Color{R: 1, A: 1}),
		"tile.aereg": asset.EncodeRegion(&asset.RegionArtifact{Page: page, W: 16, H: 16, PageW: 32, PageH: 32}),
	})
	idx := asset.NewIndex()
	idx.Put(asset.Entry{ID: page, Kind: asset.ArtAtlasPage, Artifacts: []asset.Artifact{{Kind: asset.ArtAtlasPage, Path: "cache://page.aetex"}}})
	idx.Put(asset.Entry{ID: region, Kind: asset.ArtSpriteRegion, Artifacts: []asset.Artifact{{Kind: asset.ArtSpriteRegion, Path: "cache://tile.aereg"}}})
	l, _ := newLoader(t)
	if err := l.Mount(idx, files); err != nil {
		t.Fatal(err)
	}
	if _, ok := l.Texture(region); !ok {
		t.Fatal("primary sprite region must resolve")
	}
	if _, ok := l.MaterialTexture(region); ok {
		t.Fatal("secondary sampler silently sampled the entire neighboring atlas page")
	}
	for _, id := range []ids.AssetID{page, asset.BuiltinWhite} {
		if _, ok := l.MaterialTexture(id); !ok {
			t.Fatal("complete texture must resolve", id)
		}
	}
	doc, err := asset.ParseMaterial([]byte(fmt.Sprintf("material version=1 shader=asset(%q) { params { Noise=asset(%q) } }", ids.AssetID{Lo: 603}.String(), region.String())), "preview.aemat")
	if err != nil {
		t.Fatal(err)
	}
	sh := &material.Shader{Schema: material.NewSchema([]shader.Param{{Name: "Noise", Kind: shader.KindTexture, Slot: 1}}, 0, 0), Variants: []material.Variant{{}}}
	m, problems := doc.Build("preview", sh, l.MaterialTexture)
	if m != nil || len(problems) != 1 || problems[0].Code != material.MAT004 {
		t.Fatalf("region selection must retain reference and display MAT004, got material=%v problems=%v", m, problems)
	}
}
