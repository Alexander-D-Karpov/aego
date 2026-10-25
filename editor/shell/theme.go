package shell

import (
	"aego/editor/core/context"
	"aego/editor/core/prefs"
	"aego/engine/i18n"
	"aego/platform"
	"aego/ui/theme"
)

// themeCategory holds the theme commands; the menu shows them as Window → Theme (D345).
const themeCategory = "Theme"

func themeName(m theme.Mode) string {
	switch m {
	case theme.ModeDark:
		return i18n.Tr("editor.theme.dark", "Dark")
	case theme.ModeLight:
		return i18n.Tr("editor.theme.light", "Light")
	}
	return i18n.Tr("editor.theme.system", "System")
}

func themeTip(m theme.Mode) string {
	switch m {
	case theme.ModeDark:
		return i18n.Tr("editor.theme.dark.tip", "Use the dark editor theme")
	case theme.ModeLight:
		return i18n.Tr("editor.theme.light.tip", "Use the light editor theme")
	}
	return i18n.Tr("editor.theme.system.tip", "Follow the light or dark look of the OS")
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
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.prefs.save_failed", "Cannot save the editor preferences: {err}", i18n.Str("err", err.Error())))
	}
}

func (a *App) applyTheme() {
	a.ui.Theme = theme.For(a.prefs.Theme, a.systemDark())
}
