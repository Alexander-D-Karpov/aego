package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"aego/engine/i18n"
)

// Every character of every catalog has a glyph in each UI and code atlas, at every size and scale (D348).
func TestCatalogGlyphs(t *testing.T) {
	root := filepath.Join("..", "..")
	cats, _ := filepath.Glob(filepath.Join(root, engineDir, "*.aetext"))
	mods, _ := filepath.Glob(filepath.Join(root, "modules", "*", "res", "i18n", "*.aetext"))
	cats = append(cats, mods...)
	need := map[rune]string{}
	for _, f := range cats {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		c, err := i18n.Parse(data, f)
		if err != nil {
			t.Fatal(err)
		}
		for k, m := range c.Msgs {
			texts := []string{m.Text}
			for _, p := range m.Plural {
				texts = append(texts, p)
			}
			for _, s := range texts {
				for _, r := range s {
					if r > ' ' {
						need[r] = k
					}
				}
			}
		}
	}
	atlases, _ := filepath.Glob(filepath.Join(root, "engine", "res", "fonts", "ui-*.json"))
	if len(atlases) == 0 {
		t.Fatal("no ui atlases")
	}
	for _, a := range atlases {
		data, err := os.ReadFile(a)
		if err != nil {
			t.Fatal(err)
		}
		var atlas struct {
			Glyphs []struct {
				C rune `json:"c"`
			} `json:"glyphs"`
		}
		if err := json.Unmarshal(data, &atlas); err != nil {
			t.Fatal(err)
		}
		have := map[rune]bool{}
		for _, g := range atlas.Glyphs {
			have[g.C] = true
		}
		for r, k := range need {
			if !have[r] {
				t.Errorf("%s has no glyph for %q (U+%04X), used by %s", filepath.Base(a), r, r, k)
			}
		}
	}
}
