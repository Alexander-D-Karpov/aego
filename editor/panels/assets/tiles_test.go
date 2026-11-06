package assets

import (
	"testing"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/code/highlight"
	"aego/ui/theme"
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

func TestLightFolderSheetsContrastWithPanel(t *testing.T) {
	light, dark := theme.Light(), theme.Editor()
	fill, border := folderSheetColors(&light)
	if theme.Contrast(fill, light.Panel) < 1.2 || theme.Contrast(border, fill) < 1.5 {
		t.Fatal("light folder sheets must be distinguishable from the panel and each other")
	}
	fill, border = folderSheetColors(&dark)
	if fill != dark.CardBg || border != dark.Border {
		t.Fatal("dark folder sheet colors changed")
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
