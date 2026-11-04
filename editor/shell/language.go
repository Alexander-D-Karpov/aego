package shell

import (
	"strings"

	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/prefs"
	"aego/engine/i18n"
	"aego/platform"
	"aego/ui/widgets"
)

// languageCategory holds the language commands; the menu shows them as Window → Language (D348).
const languageCategory = "Language"

// languageNames are shown in their own language so a reader finds theirs.
var languageNames = map[string]string{i18n.English: "English", i18n.Russian: "Русский"}

func languageTitle(pref string) string {
	if n, ok := languageNames[pref]; ok {
		return n
	}
	return i18n.Tr("editor.language.auto", "Auto")
}

// menuTitle is the label of a menu category; categories stay English inside the registry.
func menuTitle(cat string) string {
	switch cat {
	case toolsMenu:
		return i18n.Tr("editor.menu.tools", "Tools")
	case "File":
		return i18n.Tr("editor.menu.file", "File")
	case "Edit":
		return i18n.Tr("editor.menu.edit", "Edit")
	case "Create":
		return i18n.Tr("editor.menu.create", "Create")
	case "Scene":
		return i18n.Tr("editor.menu.scene", "Scene")
	case "Play":
		return i18n.Tr("editor.menu.play", "Play")
	case "Assets":
		return i18n.Tr("editor.menu.assets", "Assets")
	case "Window":
		return i18n.Tr("editor.menu.window", "Window")
	case "Help":
		return i18n.Tr("editor.menu.help", "Help")
	case "Scripts":
		return i18n.Tr("editor.menu.scripts", "Scripts")
	case "Animation":
		return i18n.Tr("editor.menu.animation", "Animation")
	case "Nodes":
		return i18n.Tr("editor.menu.nodes", "Nodes")
	case "Panels":
		return i18n.Tr("editor.menu.panels", "Panels")
	case "Vectors":
		return i18n.Tr("editor.menu.vectors", "Vectors")
	case "Physics":
		return i18n.Tr("editor.menu.physics", "Physics")
	case "Audio":
		return i18n.Tr("editor.menu.audio", "Audio")
	case "Camera":
		return i18n.Tr("editor.menu.camera", "Camera")
	case "Gameplay":
		return i18n.Tr("editor.menu.gameplay", "Gameplay")
	case "Input":
		return i18n.Tr("editor.menu.input", "Input")
	case "Debug":
		return i18n.Tr("editor.menu.debug", "Debug")
	case themeCategory:
		return i18n.Tr("editor.menu.theme", "Theme")
	case languageCategory:
		return i18n.Tr("editor.menu.language", "Language")
	}
	return cat
}

// subLabel drops the "Theme: " or "Language: " prefix of a command title inside its submenu.
func subLabel(cat, title string) string {
	return strings.TrimPrefix(title, menuTitle(cat)+": ")
}

// applyLanguage sets the editor language from AEGO_LANG, the preferences or the OS (D348).
func (a *App) applyLanguage() {
	widgets.Tr = i18n.Lookup
	var locales func() []string
	if !a.eng.Config().Headless {
		if l, ok := a.eng.Window().(platform.Localer); ok {
			locales = l.PreferredLocales
		}
	}
	i18n.Apply(a.prefs.Language, locales)
}

func (a *App) LanguagePref() string {
	if a.prefs.Language == "" {
		return i18n.Auto
	}
	return a.prefs.Language
}

// SetLanguagePref saves the choice; panels read Tr every frame, so it applies at once.
func (a *App) SetLanguagePref(pref string) {
	if a.LanguagePref() == pref {
		return
	}
	a.prefs.Language = pref
	a.applyLanguage()
	if err := prefs.Save(a.opt.PrefsDir, a.prefs); err != nil {
		a.ctx.Notify(context.LevelWarn, i18n.Note("editor.prefs.save_failed", "Cannot save the editor preferences: {err}", i18n.Str("err", err.Error())))
	}
}

func (a *App) registerLanguageCommands(reg *command.Registry) error {
	for i, pref := range []string{i18n.Auto, i18n.English, i18n.Russian} {
		if err := reg.Register(command.Desc{
			ID:       "window.language." + pref,
			Name:     "Language: " + languageTitle(pref),
			Category: languageCategory,
			Order:    i,
			Label:    func() string { return menuTitle(languageCategory) + ": " + languageTitle(pref) },
			Checked:  func() bool { return a.LanguagePref() == pref },
			Run: func() error {
				a.SetLanguagePref(pref)
				return nil
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
