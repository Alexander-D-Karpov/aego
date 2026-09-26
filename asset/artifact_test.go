package asset

import (
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
