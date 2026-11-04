package asset

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"slices"
	"testing"

	"aego/core/ids"
	"aego/gpu"
)

func tryTestBundle(t testing.TB) *TryArtifacts {
	t.Helper()
	albedo, normal, mat := ids.AssetID{Lo: 1}, ids.AssetID{Lo: 2}, ids.AssetID{Lo: 3}
	d, err := ParseMaterial([]byte(`material version=1 shader=asset("`+BuiltinShaderLit.String()+`") { params { NormalMap=asset("`+normal.String()+`") NormalStrength=0.75 } }`), "demo.aemat")
	if err != nil {
		t.Fatal(err)
	}
	tex := func(semantic TextureSemantic, px []byte) TryArtifact {
		return TryArtifact{Kind: ArtTexture, Data: EncodeTexture(&TextureArtifact{Width: 1, Height: 1, Format: gpu.TexRGBA8, Semantic: semantic, Pixels: px})}
	}
	return &TryArtifacts{SourceHash: sha256.Sum256([]byte("fixture source+meta")), Assets: []TryAsset{
		{ID: albedo, Kind: ArtTexture, Name: "albedo", Deps: []ids.AssetID{normal}, Artifacts: []TryArtifact{tex(TextureColor, []byte{180, 180, 180, 255}), {Kind: ArtSpriteRegion, Data: EncodeRegion(&RegionArtifact{Page: albedo, Normal: normal, W: 1, H: 1, PageW: 1, PageH: 1})}}},
		{ID: normal, Kind: ArtTexture, Name: "normal", Artifacts: []TryArtifact{tex(TextureNormal, []byte{128, 180, 240, 255})}},
		{ID: mat, Kind: ArtMaterial, Name: "lit", Deps: d.Deps(), Artifacts: []TryArtifact{{Kind: ArtMaterial, Data: EncodeMaterial(d)}}},
	}}
}

func TestTryArtifactsCanonicalRoundTrip(t *testing.T) {
	b := tryTestBundle(t)
	encoded, err := EncodeTryArtifacts(b)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(b.Assets)
	for i := range b.Assets {
		slices.Reverse(b.Assets[i].Deps)
		slices.Reverse(b.Assets[i].Artifacts)
	}
	again, err := EncodeTryArtifacts(b)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("iteration order changed the manifest", err)
	}
	decoded, err := DecodeTryArtifacts(encoded)
	if err != nil {
		t.Fatal(err)
	}
	back, err := EncodeTryArtifacts(decoded)
	if err != nil || !bytes.Equal(encoded, back) {
		t.Fatal("roundtrip changed manifest", err)
	}
	if decoded.SourceHash != b.SourceHash || len(decoded.Assets) != 3 {
		t.Fatal("lost source manifest or resources")
	}
	for _, entry := range decoded.Assets {
		if entry.Kind == ArtMaterial {
			d, err := DecodeMaterial(entry.Artifacts[0].Data, entry.Name)
			if err != nil || d.Textures["NormalMap"] != (ids.AssetID{Lo: 2}) {
				t.Fatal("lost prepared normal material", err)
			}
		}
	}
}

func TestTryArtifactsRejectBrokenClosureAndKinds(t *testing.T) {
	cases := map[string]func(*TryArtifacts){
		"zero source hash":    func(b *TryArtifacts) { b.SourceHash = [32]byte{} },
		"duplicate":           func(b *TryArtifacts) { b.Assets = append(b.Assets, b.Assets[0]) },
		"builtin overwrite":   func(b *TryArtifacts) { b.Assets[0].ID = BuiltinMaterialLit },
		"missing dependency":  func(b *TryArtifacts) { b.Assets[0].Deps = nil },
		"missing resource":    func(b *TryArtifacts) { b.Assets = b.Assets[:1] },
		"cycle":               func(b *TryArtifacts) { b.Assets[1].Deps = []ids.AssetID{b.Assets[0].ID} },
		"wrong artifact kind": func(b *TryArtifacts) { b.Assets[0].Artifacts[0].Kind = ArtMaterial },
		"truncated texture":   func(b *TryArtifacts) { b.Assets[0].Artifacts[0].Data = b.Assets[0].Artifacts[0].Data[:20] },
		"too many resources":  func(b *TryArtifacts) { b.Assets = make([]TryAsset, MaxTryAssets+1) },
		"too big artifact":    func(b *TryArtifacts) { b.Assets[0].Artifacts[0].Data = make([]byte, MaxTryArtifactBytes+1) },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			b := tryTestBundle(t)
			alter(b)
			if _, err := EncodeTryArtifacts(b); err == nil {
				t.Fatal("accepted broken bundle")
			}
		})
	}
}

func TestTryArtifactsChecksBudgetsBeforeAllocatingAndHashesAllContent(t *testing.T) {
	data, err := EncodeTryArtifacts(tryTestBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	rehash := func(b []byte) []byte { h := tryArtifactsHash(b); copy(b[44:76], h[:]); return b }
	cases := map[string]func([]byte) []byte{
		"source tampered":     func(b []byte) []byte { b[12] ^= 1; return b },
		"body tampered":       func(b []byte) []byte { b[len(b)-1] ^= 1; return b },
		"asset hash tampered": func(b []byte) []byte { b[len(b)-1] ^= 1; return rehash(b) },
		"count":               func(b []byte) []byte { binary.LittleEndian.PutUint32(b[8:12], 0xffffffff); return rehash(b) },
		"unknown kind":        func(b []byte) []byte { b[tryArtifactsHeader+16] = 255; return rehash(b) },
		"name length": func(b []byte) []byte {
			binary.LittleEndian.PutUint16(b[tryArtifactsHeader+17:], 65535)
			return rehash(b)
		},
		"trailing": func(b []byte) []byte { return rehash(append(b, 0)) },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTryArtifacts(alter(bytes.Clone(data))); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
	for n := 0; n < len(data); n++ {
		if _, err := DecodeTryArtifacts(data[:n]); err == nil {
			t.Fatalf("accepted prefix %d", n)
		}
	}
	if _, err := DecodeTryArtifacts(make([]byte, MaxTryArtifactsBytes+1)); err == nil {
		t.Fatal("accepted oversized manifest")
	}
}

func TestTryArtifactsAtlasPage(t *testing.T) {
	b := tryTestBundle(t)
	b.Assets = b.Assets[1:2]
	b.Assets[0].Kind = ArtAtlasPage
	b.Assets[0].Artifacts[0].Kind = ArtAtlasPage
	data, err := EncodeTryArtifacts(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeTryArtifacts(data); err != nil {
		t.Fatal(err)
	}
}

func FuzzTryArtifacts(f *testing.F) {
	data, err := EncodeTryArtifacts(tryTestBundle(f))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte("AETB"))
	f.Fuzz(func(t *testing.T, b []byte) {
		decoded, err := DecodeTryArtifacts(b)
		if err != nil {
			return
		}
		encoded, err := EncodeTryArtifacts(decoded)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeTryArtifacts(encoded); err != nil {
			t.Fatal(err)
		}
	})
}
