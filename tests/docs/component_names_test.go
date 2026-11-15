//go:build !aego_runtime

package docs_test

import (
	"slices"
	"testing"

	"aego/engine/i18n"
)

func TestCameraOutputSelectorUsesSharedNamesAndRetainsSearchAliases(t *testing.T) {
	r, _ := build(t)
	const id = "camera.CameraBrain2D"
	e, ok := r.Get(id)
	if !ok || e.Title != "Camera Output Selector 2D" {
		t.Fatalf("component technical ID or public English title changed incorrectly: %+v", e)
	}
	lang := i18n.Default().Language()
	for _, name := range []string{id, "Camera Output Selector 2D", "Выбор выхода камеры 2D", "Camera Brain 2D", "Мозг камеры 2D"} {
		if !slices.Contains(e.SearchNames, name) {
			t.Errorf("shared name %q missing from component search names: %v", name, e.SearchNames)
		}
		got := r.Search(name, 1)
		if len(got) != 1 || got[0].ID != id {
			t.Errorf("%q did not find the unchanged technical ID: %+v", name, got)
		}
	}
	if i18n.Default().Language() != lang {
		t.Fatal("building multilingual search changed the active UI language")
	}
}
