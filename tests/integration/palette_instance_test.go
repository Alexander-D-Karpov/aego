package integration

import (
	"testing"

	"aego/platform/input"
)

func TestPaletteAddsToPrefabInstance(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	sc := ctx.Scene()
	h := sc.Find(sc.Root(), "EnemyLeft")
	ctx.Select(sc.ID(h))
	s.Step()
	push(t, s, key(input.KeyA, input.PrimaryMod()|input.ModShift)...)
	pal := s.App.AddPalette()
	if !pal.Active() {
		t.Fatal("not open")
	}
	push(t, s, typed("lifetime")...)
	if i := pal.Palette().Selected(); i >= 0 {
		it := pal.Palette().Item(i)
		t.Logf("selected %q disabled %q", it.Label, it.Disabled)
	} else {
		t.Logf("nothing selected; items %d", len(pal.Palette().Items()))
		for _, it := range pal.Palette().Items() {
			if it.Label == "Lifetime" {
				t.Logf("lifetime disabled %q", it.Disabled)
			}
		}
	}
	push(t, s, key(input.KeyEnter, 0)...)
	td, _ := sc.Registry().ByName("gameplay.Lifetime")
	if _, ok := sc.GetByType(h, td.ID); !ok {
		t.Fatal("no Lifetime on the instance")
	}
}

func TestInspectorAddsToPrefabInstance(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	sc := ctx.Scene()
	h := sc.Find(sc.Root(), "EnemyLeft")
	ctx.Select(sc.ID(h))
	s.App.Workspace().Show("inspector")
	for range 5 {
		s.Step()
	}
	s.App.Inspector().OpenAdd("lifetime")
	for range 3 {
		s.Step()
	}
	pal := s.App.AddPalette()
	if !pal.Active() {
		t.Fatal("inspector did not open the palette")
	}
	if i := pal.Palette().Selected(); i >= 0 {
		it := pal.Palette().Item(i)
		t.Logf("selected %q disabled %q", it.Label, it.Disabled)
	} else {
		t.Log("nothing selected")
	}
	push(t, s, key(input.KeyEnter, 0)...)
	td, _ := sc.Registry().ByName("gameplay.Lifetime")
	if _, ok := sc.GetByType(h, td.ID); !ok {
		t.Fatal("no Lifetime on the instance")
	}
}
