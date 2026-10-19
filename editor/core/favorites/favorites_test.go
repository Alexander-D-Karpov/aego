package favorites

import (
	"testing"

	"aego/core/ids"
)

func TestToggleSaveLoad(t *testing.T) {
	root := t.TempDir()
	s := Load(root)
	items := []Item{
		{Kind: Asset, Asset: ids.AssetID{Hi: 1, Lo: 2}, Label: "hero.png"},
		{Kind: Node, Scene: "scenes/main.aescene", Node: ids.NewNodeID(), Label: "Player"},
		{Kind: Command, Command: "play.start", Label: "Play"},
	}
	for _, it := range items {
		if added, err := s.Toggle(it); err != nil || !added {
			t.Fatalf("toggle %v: added=%v err=%v", it, added, err)
		}
	}
	r := Load(root)
	if r.Err() != nil || len(r.Items()) != 3 {
		t.Fatalf("loaded %d items, err %v", len(r.Items()), r.Err())
	}
	for _, it := range items {
		if !r.Has(it) {
			t.Fatalf("lost %v", it)
		}
	}
	if added, _ := r.Toggle(items[1]); added || r.Has(items[1]) {
		t.Fatal("a second toggle removes the item")
	}
}
