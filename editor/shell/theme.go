package shell

import (
	"strings"

	"aego/editor/core/context"
	"aego/editor/core/prefs"
	"aego/platform"
	"aego/ui/theme"
)

// themeCategory holds the theme commands; the menu shows them as Window → Theme (D345).
const themeCategory = "Theme"

var themeTips = map[theme.Mode]string{
	theme.ModeSystem: "Follow the light or dark look of the OS",
	theme.ModeDark:   "Use the dark editor theme",
	theme.ModeLight:  "Use the light editor theme",
}

func themeLabel(name string) string {
	return strings.TrimPrefix(name, themeCategory+": ")
}

func (a *App) systemDark() bool {
	if st, ok := a.eng.Window().(platform.SystemThemer); ok {
		return st.SystemTheme() != platform.ThemeLight
	}
	return true
}

func (a *App) ThemeMode() theme.Mode {
	return a.prefs.Theme
}

// SetThemeMode saves the mode and swaps the theme at the start of the next frame (D345).
func (a *App) SetThemeMode(m theme.Mode) {
	if a.prefs.Theme == m {
		return
	}
	a.prefs.Theme = m
	a.retheme = true
	if err := prefs.Save(a.opt.PrefsDir, a.prefs); err != nil {
		a.ctx.Notify(context.LevelWarn, "cannot save the editor preferences: "+err.Error())
	}
}

func (a *App) applyTheme() {
	a.ui.Theme = theme.For(a.prefs.Theme, a.systemDark())
}
