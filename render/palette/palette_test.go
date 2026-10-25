package palette

import "testing"

func TestBuiltin(t *testing.T) {
	ids := map[string]bool{}
	for _, p := range Builtin() {
		if p.ID == "" || p.Name == "" || p.Group == "" {
			t.Fatalf("palette %+v lacks an id, name or group", p)
		}
		if ids[p.ID] {
			t.Fatalf("duplicate palette id %q", p.ID)
		}
		ids[p.ID] = true
		if len(p.Colors) < 4 || len(p.Colors) > 64 {
			t.Fatalf("%s has %d colors", p.ID, len(p.Colors))
		}
		seen := map[uint32]bool{}
		for _, c := range p.Colors {
			if c > 0xffffff {
				t.Fatalf("%s: color %x is not 0xRRGGBB", p.ID, c)
			}
			if seen[c] {
				t.Fatalf("%s repeats %06x", p.ID, c)
			}
			seen[c] = true
		}
		if got := p.Linear(); len(got) != len(p.Colors) || got[0].A != 1 {
			t.Fatalf("%s: Linear gave %v", p.ID, got)
		}
	}
	if _, ok := ByID("pico8"); !ok {
		t.Fatal("ByID misses pico8")
	}
}

func TestRamps(t *testing.T) {
	g, _ := ByID("grays")
	if g.Colors[0] != 0 || g.Colors[len(g.Colors)-1] != 0xffffff {
		t.Fatalf("grays = %06x", g.Colors)
	}
	h, _ := ByID("hues")
	if h.Colors[0]>>16 != 0xf2 {
		t.Fatalf("first hue = %06x, want red", h.Colors[0])
	}
}
