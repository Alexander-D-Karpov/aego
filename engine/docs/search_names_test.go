//go:build !aego_runtime

package docs

import (
	"slices"
	"testing"
	"testing/fstest"

	"aego/engine/module"
)

func TestSearchNamesFromPagesOverlaysAndTranslations(t *testing.T) {
	r := &Registry{byID: map[string]int{}, meta: map[string]pageMeta{}}
	r.page("engine", "page.md", "page.md", record{id: "guide", title: "Effects", searchNames: []string{"bloom"}})
	r.add(Entry{ID: "render2d.PostProcess", Kind: Type, Title: "Post Process"})
	r.page("render2d", "effect.md", "effect.md", record{id: "render2d.PostProcess", searchNames: []string{"glow"}})
	src := module.DocSource{Owner: "engine", FS: fstest.MapFS{"page.ru.md": {Data: []byte("---\nid: guide\nsearch_names: [блум, свечение]\n---\n# Эффекты\n")}}}
	r.translations(src, []string{"page.ru.md"})
	if !slices.Equal(r.entries[0].SearchNames, []string{"bloom", "блум", "свечение"}) || len(r.entries[0].Keys) != 0 {
		t.Fatalf("search aliases changed hover keys or were lost: %+v", r.entries[0])
	}
	for query, want := range map[string]string{"bloom": "guide", "блум": "guide", "свечение": "guide", "glow": "render2d.PostProcess"} {
		got := r.Search(query, 1)
		if len(got) != 1 || got[0].ID != want {
			t.Errorf("Search(%q) = %v", query, got)
		}
	}
	if _, err := parseRecords("---\nid: guide\nsearch_names: not-a-list\n---\n"); err == nil {
		t.Fatal("invalid alias list accepted")
	}
}
