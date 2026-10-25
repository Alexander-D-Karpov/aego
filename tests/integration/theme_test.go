package integration

import (
	"path/filepath"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/prefs"
	"aego/project"
	"aego/ui/theme"
)

// Theme: Light swaps the theme on the next frame, is saved in prefs.aetext and is what the next session starts with (D358).
func TestEditorThemeCommand(t *testing.T) {
	t.Setenv("AEGO_EDITOR_THEME", "")
	cfgDir := t.TempDir()
	open := func() *edit.Session {
		p, err := project.Load(copyExample(t, "platformer"))
		if err != nil {
			t.Fatal(err)
		}
		s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop(), Recent: filepath.Join(cfgDir, "recent.aetext")})
		if err != nil {
			t.Fatal(err)
		}
		s.Step()
		return s
	}
	s := open()
	ctx := s.App.Context()
	if !ctx.UI.Theme.Dark {
		t.Fatal("System without an OS theme must start dark")
	}
	if err := ctx.Commands.Run("window.theme.light"); err != nil {
		t.Fatal(err)
	}
	s.Step()
	if ctx.UI.Theme.Dark {
		t.Fatal("Theme: Light did not swap the theme")
	}
	if d, _ := ctx.Commands.Get("window.theme.light"); !d.Checked() {
		t.Fatal("Theme: Light is not checked in the menu")
	}
	s.Close()
	if p, err := prefs.Load(cfgDir); err != nil || p.Theme != theme.ModeLight {
		t.Fatalf("prefs.aetext holds %v, %v", p.Theme, err)
	}
	s = open()
	defer s.Close()
	if s.App.Context().UI.Theme.Dark {
		t.Fatal("the next session ignores the saved theme")
	}
}
