package integration

import (
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/engine/i18n"
	"aego/engine/settings"
	"aego/schema"
	"aego/script/bind"
	"aego/script/diag"
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

// Component names, field labels, enum values, settings and script diagnostics come from the data catalog (11g2, D348).
func TestRegistryTextsInRussian(t *testing.T) {
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.New(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	body, ok := types.Schema().ByName("physics.Body2D")
	if !ok {
		t.Fatal("no physics.Body2D")
	}
	motion, _ := types.Schema().ByName("gameplay.Motion2D")
	b := i18n.Default()
	b.SetLanguage(i18n.Russian)
	defer b.SetLanguage(i18n.English)
	typ := &body.Fields[0]
	for i := range body.Fields {
		if body.Fields[i].Name == "type" {
			typ = &body.Fields[i]
		}
	}
	for got, want := range map[string]string{
		schema.Title(body):               "Тело 2D",
		schema.Title(motion):             "Движение 2D",
		schema.FieldLabel(body, typ):     "Тип",
		schema.EnumLabel(typ, 0):         "статическое",
		settings.CategoryText("Display"): "Экран",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if h, _ := diag.Localized("AES2001"); h.What != "Это имя не объявлено в данной области видимости." {
		t.Errorf("AES2001 in Russian: %q", h.What)
	}
	b.SetLanguage(i18n.English)
	if schema.Title(body) != "Rigid Body 2D" || schema.EnumLabel(typ, 0) != "static" {
		t.Error("English texts changed")
	}
}
