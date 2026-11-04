package playground

import (
	"crypto/sha256"
	"testing"

	"aego/asset"
	"aego/asset/loader"
	"aego/core/ids"
	"aego/core/log"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/texture"
)

func preparedTexture(t *testing.T, pixel byte) []byte {
	t.Helper()
	a, n := ids.AssetID{Lo: 1}, ids.AssetID{Lo: 2}
	b, err := asset.EncodeTryArtifacts(&asset.TryArtifacts{SourceHash: sha256.Sum256([]byte{pixel}), Assets: []asset.TryAsset{
		{ID: a, Kind: asset.ArtTexture, Name: "albedo", Deps: []ids.AssetID{n}, Artifacts: []asset.TryArtifact{
			{Kind: asset.ArtTexture, Data: asset.EncodeTexture(&asset.TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Pixels: []byte{pixel, pixel, pixel, 255}})},
			{Kind: asset.ArtSpriteRegion, Data: asset.EncodeRegion(&asset.RegionArtifact{Page: a, Normal: n, W: 1, H: 1, PageW: 1, PageH: 1})},
		}},
		{ID: n, Kind: asset.ArtTexture, Name: "normal", Artifacts: []asset.TryArtifact{{Kind: asset.ArtTexture, Data: asset.EncodeTexture(&asset.TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Semantic: asset.TextureNormal, Pixels: []byte{128, 128, 255, 255}})}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMountTryPreparedResourcesAtomicValidationAndLifetime(t *testing.T) {
	d := null.New()
	l := loader.New(d, texture.NewUploader(d), log.Nop())
	defer l.Close()
	if err := MountTry(l, nil, preparedTexture(t, 180)); err != nil {
		t.Fatal(err)
	}
	a := ids.AssetID{Lo: 1}
	before, ok := l.Texture(a)
	if !ok {
		t.Fatal("prepared albedo not loaded")
	}
	if id, n, ok := l.NormalFor(a); !ok || id != (ids.AssetID{Lo: 2}) || n.Texture.IsZero() {
		t.Fatal("prepared normal mapping not loaded")
	}
	generation := l.Generation()
	bad := preparedTexture(t, 190)
	bad[len(bad)-1] ^= 1
	if err := MountTry(l, nil, bad); err == nil {
		t.Fatal("corrupted payload accepted")
	}
	kept, ok := l.Texture(a)
	if !ok || kept != before || l.Generation() != generation {
		t.Fatal("failed validation changed previous mount")
	}
	for i := 0; i < 20; i++ {
		if err := MountTry(l, nil, preparedTexture(t, byte(i))); err != nil {
			t.Fatal(err)
		}
		next, ok := l.Texture(a)
		if !ok || next.Texture == before.Texture {
			t.Fatal("reused ID kept stale resident texture")
		}
		before = next
		if _, _, ok := l.NormalFor(a); !ok {
			t.Fatal("new session lost normal")
		}
		if stats := d.ResourceStats(); stats.Textures != 2 {
			t.Fatalf("session switch leaked textures: %+v", stats)
		}
	}
	if err := MountTry(l, nil); err != nil {
		t.Fatal(err)
	}
	if l.Index().Len() != 0 || l.Resident() != 0 || d.ResourceStats().Textures != 0 {
		t.Fatal("session unmount leaked resources")
	}
}
