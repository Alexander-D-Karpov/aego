//go:build !aego_runtime

package docs_test

import (
	"strings"
	"testing"

	"aego/app/docstool"
)

func TestValueTypeCatalogTranslationsReachDocumentation(t *testing.T) {
	r, err := docstool.Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{
		"aego/audio.PlayOptions",
		"aego/camera.Pose",
		"aego/data.AudioEvent",
		"aego/material.Vec4",
		"aego/physics.Contact",
		"aego/physics.RayHit",
		"engine.MaterialParameter",
		"particles.ColorKey",
		"render2d.PostEffect",
		"render2d.PostParameter",
	} {
		e, ok := r.Get(id)
		if !ok || e.Body == "" || e.Translations["ru"].Body == "" || e.Translations["ru"].Body == e.Body {
			t.Errorf("%s is missing its localized value-type description", id)
		}
	}
	e, _ := r.Get("engine.MaterialParameter")
	const paragraph = "Хранит одно явное типизированное переопределение материала узла."
	if n := strings.Count(e.Translations["ru"].Body, paragraph); n != 1 {
		t.Errorf("authored MaterialParameter translation occurs %d times: %q", n, e.Translations["ru"].Body)
	}
}
