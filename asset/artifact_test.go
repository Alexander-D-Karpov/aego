package asset

import (
	"encoding/binary"
	"testing"

	"aego/core/ids"
	"aego/gpu"
)

func TestTextureArtifactRoundTrip(t *testing.T) {
	pixels := make([]byte, 4*4*4)
	for i := range pixels {
		pixels[i] = byte(i)
	}
	src := &TextureArtifact{Width: 4, Height: 4, Format: gpu.TexSRGBA8, Filter: gpu.FilterLinear, Mips: true, Pixels: pixels}
	back, err := DecodeTexture(EncodeTexture(src))
	if err != nil {
		t.Fatal(err)
	}
	if back.Width != 4 || back.Height != 4 || back.Format != gpu.TexSRGBA8 || back.Filter != gpu.FilterLinear || !back.Mips {
		t.Fatalf("header lost: %+v", back)
	}
	if string(back.Pixels) != string(pixels) {
		t.Fatal("pixels lost")
	}
}

func TestTextureArtifactRejectsTruncated(t *testing.T) {
	src := &TextureArtifact{Width: 8, Height: 8, Pixels: make([]byte, 16)}
	if _, err := DecodeTexture(EncodeTexture(src)); err == nil {
		t.Fatal("truncated pixel data must be rejected")
	}
	if _, err := DecodeTexture([]byte("short")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}

func TestRegionArtifactRoundTrip(t *testing.T) {
	page := ids.NewAssetID()
	src := &RegionArtifact{Page: page, X: 3, Y: 5, W: 16, H: 32, PageW: 256, PageH: 256, Filter: gpu.FilterNearest}
	back, err := DecodeRegion(EncodeRegion(src))
	if err != nil {
		t.Fatal(err)
	}
	if *back != *src {
		t.Fatalf("round trip lost data: %+v vs %+v", back, src)
	}
}

func TestTrimRegionArtifactAndLegacyVersion(t *testing.T) {
	src := &RegionArtifact{X: 2, Y: 3, W: 3, H: 2, PageW: 64, PageH: 64, SourceW: 8, SourceH: 6, TrimX: 2, TrimY: 1}
	data := EncodeRegion(src)
	r, err := DecodeRegion(data)
	if err != nil || *r != *src {
		t.Fatalf("trim roundtrip %+v %v", r, err)
	}
	legacy := append([]byte(nil), data[:48]...)
	legacy[4] = 1
	r, err = DecodeRegion(legacy)
	if err != nil || r.SourceW != 0 || r.TrimX != 0 {
		t.Fatalf("legacy artifact %+v %v", r, err)
	}
	src.TrimX = 7
	if _, err := DecodeRegion(EncodeRegion(src)); err == nil {
		t.Fatal("invalid source trim accepted")
	}
}

func TestSurfaceArtifactVersionsAndSemantic(t *testing.T) {
	normalID := ids.NewAssetID()
	src := &RegionArtifact{Page: ids.NewAssetID(), Normal: normalID, W: 4, H: 3, PageW: 32, PageH: 32, SourceW: 8, SourceH: 8, TrimX: 2, TrimY: 1}
	encoded := EncodeRegion(src)
	back, err := DecodeRegion(encoded)
	if err != nil || *src != *back {
		t.Fatalf("normal/trim mapping did not round trip: %+v %v", back, err)
	}
	legacy := append([]byte(nil), encoded[:56]...)
	binary.LittleEndian.PutUint32(legacy[4:8], 2)
	back, err = DecodeRegion(legacy)
	if err != nil || !back.Normal.IsZero() || back.SourceW != src.SourceW {
		t.Fatalf("AERG2 compatibility: %+v %v", back, err)
	}
	if _, err := DecodeRegion(encoded[:60]); err == nil {
		t.Fatal("truncated AERG3 normal reference accepted")
	}
	texture := &TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Semantic: TextureNormal, Pixels: []byte{200, 50, 255, 0}}
	data := EncodeTexture(texture)
	tex, err := DecodeTexture(data)
	if err != nil || tex.Semantic != TextureNormal || string(tex.Pixels) != string(texture.Pixels) {
		t.Fatalf("normal semantic changed: %+v %v", tex, err)
	}
	data[16] = byte(gpu.TexSRGBA8)
	if _, err := DecodeTexture(data); err == nil {
		t.Fatal("normal artifact with sRGB storage accepted")
	}
	data = EncodeTexture(texture)
	binary.LittleEndian.PutUint32(data[4:8], 1)
	tex, err = DecodeTexture(data)
	if err != nil || tex.Semantic != TextureColor {
		t.Fatalf("legacy AETX1 did not preserve color contract: %+v %v", tex, err)
	}
	data = EncodeTexture(texture)
	binary.LittleEndian.PutUint32(data[8:12], ^uint32(0))
	if _, err := DecodeTexture(data); err == nil {
		t.Fatal("overflowing dimensions accepted")
	}
}
