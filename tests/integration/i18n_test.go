package integration

import (
	"testing"

	"aego/engine/i18n"
)

// Built-in modules register their catalogs through Registry.Messages, under i18n.<module>.* (D348).
func TestModuleCatalogsLoad(t *testing.T) {
	workspaceSession(t)
	b := i18n.Default()
	b.SetLanguage(i18n.Russian)
	defer b.SetLanguage(i18n.English)
	for key, en := range map[string]string{
		"i18n.physics.gravity":   "Gravity",
		"i18n.input.add_action":  "Add Action",
		"i18n.audio.mixer.title": "Audio Mixer",
	} {
		if got := i18n.Lookup(key, en); got == en {
			t.Errorf("%s has no Russian text after the session loaded the module catalogs", key)
		}
	}
}
