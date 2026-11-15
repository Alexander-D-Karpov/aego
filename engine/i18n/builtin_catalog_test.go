package i18n

import (
	"strings"
	"testing"
)

func TestEmbeddedValueTypeTranslations(t *testing.T) {
	readCatalog := func(path string) *Catalog {
		t.Helper()
		data, err := messages.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		c, err := Parse(data, path)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	en, ru := readCatalog("data/en.aetext"), readCatalog("data/ru.aetext")
	b := engineBundle()
	for _, key := range []string{
		"api.aego/audio.PlayOptions",
		"api.aego/camera.Pose",
		"api.aego/data.AudioEvent",
		"api.aego/material.Vec4",
		"api.aego/physics.Contact",
		"api.aego/physics.RayHit",
		"api.engine.MaterialParameter",
		"api.particles.ColorKey",
		"api.render2d.PostEffect",
		"api.render2d.PostParameter",
	} {
		english, hasEnglish := en.Msgs[key]
		russian, hasRussian := ru.Msgs[key]
		if !hasEnglish || strings.TrimSpace(english.Text) == "" || !hasRussian || strings.TrimSpace(russian.Text) == "" || russian.Text == english.Text {
			t.Errorf("%s needs distinct English and Russian descriptions", key)
			continue
		}
		if got := b.In(Russian, key, english.Text); got != russian.Text {
			t.Errorf("embedded translation %s: got %q, want %q", key, got, russian.Text)
		}
	}
}
