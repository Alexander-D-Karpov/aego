package text

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMonoAtlasGrid(t *testing.T) {
	paths, _ := filepath.Glob("../../engine/res/fonts/ui-mono-*.json")
	if len(paths) == 0 {
		t.Fatal("no mono atlas in engine/res/fonts, run genfont with -mono")
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var fj fontFileJSON
		if err := json.Unmarshal(data, &fj); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		f := NewFont(fj.Size, fj.Scale, fj.LineHeight, fj.Ascent)
		for _, g := range fj.Glyphs {
			f.SetGlyph(rune(g.C), FontGlyph{Advance: g.Advance})
		}
		for _, k := range fj.Kerning {
			f.SetKern(rune(k.A), rune(k.B), k.K)
		}
		adv := f.Advance('0')
		for r := rune(0x20); r <= 0x7E; r++ {
			if !f.Has(r) {
				t.Errorf("%s: missing %q", p, r)
			} else if f.Advance(r) != adv {
				t.Errorf("%s: %q advance %v, cell %v", p, r, f.Advance(r), adv)
			}
		}
		for _, r := range "→·…" {
			if !f.Has(r) {
				t.Errorf("%s: missing %q", p, r)
			}
		}
		const s = "{}[]()<>|&"
		if w := f.Measure(s).X; w != float32(len(s))*adv {
			t.Errorf("%s: %q width %v, want %v", p, s, w, float32(len(s))*adv)
		}
	}
}
