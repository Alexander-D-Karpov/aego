package command

import (
	"testing"

	"aego/platform/input"
)

func TestShortcutFormatPerOS(t *testing.T) {
	s := Key(input.KeyS, ModPrimary|ModShift)
	for goos, want := range map[string]string{
		"linux":   "Ctrl+Shift+" + input.KeyS.String(),
		"windows": "Ctrl+Shift+" + input.KeyS.String(),
		"darwin":  "⌘⇧" + input.KeyS.String(),
	} {
		if got := s.Format(goos); got != want {
			t.Fatalf("%s: got %q, want %q", goos, got, want)
		}
	}
	if (Shortcut{}).Format("linux") != "" {
		t.Fatal("empty shortcut must format to an empty string")
	}
}

func TestCategoryOrder(t *testing.T) {
	r := New()
	run := func() error { return nil }
	r.Register(Desc{ID: "b", Name: "B", Category: "File", Order: 200, Run: run})
	r.Register(Desc{ID: "a", Name: "A", Category: "File", Order: 100, Run: run})
	r.Register(Desc{ID: "x", Name: "X", Category: "Game", Run: run})
	got := r.Category("File")
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("category = %+v", got)
	}
	cats := r.Categories()
	if len(cats) != 2 || cats[0] != "File" || cats[1] != "Game" {
		t.Fatalf("categories = %v", cats)
	}
}
