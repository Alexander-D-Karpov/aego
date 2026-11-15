package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSearchIndexRetainsSharedPublicNames(t *testing.T) {
	e := fixture()
	const id = "engine.SpriteRenderer"
	var want = []string{"engine.SpriteRenderer", "Sprite Renderer", "Отрисовка спрайта", "Old Sprite Name"}
	for i := range e.Entries {
		if e.Entries[i].ID == id {
			e.Entries[i].SearchNames = want
		}
	}
	dir, _, _ := build(t, e, Options{})
	for _, rel := range []string{"search.json", "ru/search.json"} {
		raw, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatal(err)
		}
		var items []searchItem
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range items {
			if item.ID != id {
				continue
			}
			found = true
			if !slices.Equal(item.Aliases, want) {
				t.Fatalf("%s: shared component names dropped from web search: %v", rel, item.Aliases)
			}
			if item.Section != SectionReference {
				t.Fatalf("%s: stable section key was localized: %q", rel, item.Section)
			}
		}
		if !found {
			t.Fatalf("component missing from %s", rel)
		}
	}
}
