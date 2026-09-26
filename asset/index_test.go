package asset

import (
	"testing"

	"aego/core/ids"
)

func TestIndexRoundTrip(t *testing.T) {
	page := ids.NewAssetID()
	sprite := ids.NewAssetID()
	idx := NewIndex()
	idx.Put(Entry{
		ID: page, Kind: ArtAtlasPage, Name: "atlas/characters", Source: "assets/characters",
		Artifacts: []Artifact{{Kind: ArtAtlasPage, Path: "cache://p0.aetex", Hash: "abc", Size: 1024}},
	})
	idx.Put(Entry{
		ID: sprite, Kind: ArtSpriteRegion, Name: "player", Source: "assets/player.png",
		Deps:      []ids.AssetID{page},
		Artifacts: []Artifact{{Kind: ArtSpriteRegion, Path: "cache://player.aereg", Hash: "def", Size: 45}},
	})

	data := WriteIndex(idx)
	back, err := ReadIndex(data, "index.aetext")
	if err != nil {
		t.Fatal(err)
	}
	if back.Len() != 2 || back.Version != IndexVersion {
		t.Fatalf("len=%d version=%d", back.Len(), back.Version)
	}
	e, ok := back.Lookup(sprite)
	if !ok {
		t.Fatal("sprite entry missing")
	}
	if e.Name != "player" || e.Source != "assets/player.png" || len(e.Deps) != 1 || e.Deps[0] != page {
		t.Fatalf("entry = %+v", e)
	}
	a, ok := e.Artifact(ArtSpriteRegion)
	if !ok || a.Hash != "def" || a.Size != 45 {
		t.Fatalf("artifact = %+v", a)
	}
	if string(WriteIndex(back)) != string(data) {
		t.Fatal("index round trip is not canonical")
	}
}

func TestIndexDelete(t *testing.T) {
	idx := NewIndex()
	a := ids.NewAssetID()
	b := ids.NewAssetID()
	idx.Put(Entry{ID: a, Name: "a"})
	idx.Put(Entry{ID: b, Name: "b"})
	idx.Delete(a)
	if idx.Len() != 1 {
		t.Fatalf("len = %d", idx.Len())
	}
	if _, ok := idx.Lookup(a); ok {
		t.Fatal("deleted entry still resolves")
	}
	seen := 0
	idx.Each(func(*Entry) { seen++ })
	if seen != 1 {
		t.Fatalf("Each visited %d", seen)
	}
}

func TestIndexRejectsNewerVersion(t *testing.T) {
	src := []byte("index version=99 {}\n")
	if _, err := ReadIndex(src, "x"); err == nil {
		t.Fatal("a newer index version must be rejected")
	}
}
