package shell

import (
	"testing"

	"aego/engine/i18n"
)

// The menu reads Tr every frame, so a language switch shows at once (D348).
func TestMenuTitleFollowsLanguage(t *testing.T) {
	b := i18n.Default()
	t.Cleanup(func() { b.SetLanguage(i18n.English) })
	if got := menuTitle("File"); got != "File" {
		t.Fatalf("en: %q", got)
	}
	b.SetLanguage(i18n.Russian)
	if got := menuTitle("File"); got != "Файл" {
		t.Fatalf("ru: %q", got)
	}
	if got := subLabel(themeCategory, menuTitle(themeCategory)+": "+themeName(0)); got == "" || got[0] == 'T' {
		t.Fatalf("theme submenu label: %q", got)
	}
	b.SetLanguage(i18n.English)
	if got := menuTitle("Window"); got != "Window" {
		t.Fatalf("back to en: %q", got)
	}
}
