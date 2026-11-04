package asset

import (
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"slices"
	"testing"
)

func TestMaskKeepsLogicalFrameWhenRepacked(t *testing.T) {
	id, page := ids.NewAssetID(), ids.NewAssetID()
	r := RegionArtifact{Page: page, W: 2, H: 2, PageW: 8, PageH: 6, SourceW: 6, SourceH: 4, TrimX: 1, TrimY: 1, HasPivot: true, Pivot: [2]float32{.25, .75}}
	tex := TextureArtifact{Width: 8, Height: 6, Format: gpu.TexRGBA8}
	for _, loc := range [][2]int{{0, 1}, {5, 3}} {
		r.X, r.Y = loc[0], loc[1]
		tex.Pixels = make([]byte, 8*6*4)
		tex.Pixels[4*(8*r.Y+r.X)+3] = 240
		tex.Pixels[4*(8*(r.Y+1)+r.X+1)+3] = 99
		read := func(i ids.AssetID, k ArtifactKind) ([]byte, bool) {
			if i == id && k == ArtSpriteRegion {
				return EncodeRegion(&r), true
			}
			if i == page && k == ArtAtlasPage {
				return EncodeTexture(&tex), true
			}
			return nil, false
		}
		points, ok := PrepareParticleMask(id, 100, read)
		if !ok || !slices.Equal(points, []mathx.Vec2{mathx.V2(0, -.375)}) {
			t.Fatalf("packed at %v: %v", loc, points)
		}
		if empty, ok := PrepareParticleMask(id, 255, read); !ok || len(empty) != 0 {
			t.Fatal("valid empty mask reported as missing")
		}
		if _, ok := PrepareParticleMask(ids.NewAssetID(), 0, read); ok {
			t.Fatal("missing mask accepted")
		}
	}
}

func TestMaskRoleUsesSourceAlpha(t *testing.T) {
	id := ids.NewAssetID()
	raw := EncodeTexture(&TextureArtifact{Width: 2, Height: 1, Format: gpu.TexRGBA8, Semantic: TextureMask, Pixels: []byte{255, 255, 255, 0, 0, 0, 0, 200}})
	pts, ok := PrepareParticleMask(id, 128, func(k ids.AssetID, kind ArtifactKind) ([]byte, bool) { return raw, k == id && kind == ArtTexture })
	if !ok || !slices.Equal(pts, []mathx.Vec2{mathx.V2(.25, 0)}) {
		t.Fatalf("mask role lost its source alpha: %v, %v", pts, ok)
	}
}
func TestOutOfRangePreparedMaskPivotRejected(t *testing.T) {
	for _, p := range [][2]float32{{-1, 0}, {0, 2}} {
		r := RegionArtifact{Page: ids.NewAssetID(), PageW: 1, PageH: 1, W: 1, H: 1, HasPivot: true, Pivot: p}
		if _, err := DecodeRegion(EncodeRegion(&r)); err == nil {
			t.Fatalf("accepted normalized pivot %v", p)
		}
	}
}
