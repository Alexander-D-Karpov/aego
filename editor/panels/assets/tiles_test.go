package assets

import (
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/code/highlight"
)

func TestDeclName(t *testing.T) {
	for _, c := range []struct {
		line string
		name string
		kind highlight.Kind
		ok   bool
	}{
		{"const FloorNormal float = -0.5", "FloorNormal", highlight.Constant, true},
		{"func TestIsFloor() {", "TestIsFloor", highlight.Function, true},
		{"func (p *Player) Update(dt float) {", "Update", highlight.Method, true},
		{"var speed = 3", "speed", highlight.Variable, true},
		{"type Enemy struct {", "Enemy", highlight.Type, true},
		{"// comment", "", 0, false},
	} {
		name, kind, ok := declName([]string{c.line})
		if name != c.name || kind != c.kind || ok != c.ok {
			t.Errorf("%q: got %q %d %v", c.line, name, kind, ok)
		}
	}
}

func TestFolderStackIncludesScriptsAndData(t *testing.T) {
	p := New(Deps{})
	p.items = []pipeline.AssetInfo{
		{ID: ids.AssetID{Lo: 1}, Path: "scripts/player/player.aes", Kind: asset.ArtScript},
		{ID: ids.AssetID{Lo: 2}, Path: "scripts/other.aes", Kind: asset.ArtScript},
		{ID: ids.AssetID{Lo: 3}, Path: "assets/data/settings.aedata", Kind: asset.ArtData},
		{ID: ids.AssetID{Lo: 4}, Path: "assets/data/values.aetable", Kind: asset.ArtTable},
	}
	if got := p.stack("scripts/player"); len(got) != 1 || got[0] != 0 {
		t.Fatalf("script previews = %v", got)
	}
	if got := p.stack("assets/data"); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("data previews = %v", got)
	}
}

func TestArtSourceHasDistinctIcon(t *testing.T) {
	for _, tc := range []struct {
		path string
		kind asset.ArtifactKind
		want string
	}{
		{"assets/hero.aeart", asset.ArtTexture, "file.art"},
		{"assets/HERO.AEART", asset.ArtTexture, "file.art"},
		{"assets/hero.png", asset.ArtTexture, "file.texture"},
		{"assets/hero.aeart", asset.ArtAnim, "file.unknown"},
	} {
		a := &pipeline.AssetInfo{Path: tc.path, Kind: tc.kind}
		if got := assetIcon(a); got != tc.want {
			t.Errorf("assetIcon(%q, %q) = %q, want %q", tc.path, tc.kind, got, tc.want)
		}
	}
}
