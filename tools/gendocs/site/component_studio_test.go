package site

import (
	"strings"
	"testing"
)

func TestGraphicComponentsUsePinnedLocalizedStudioExamples(t *testing.T) {
	base := "https://aeditor.akarpov.ru/r/verified-commit-12/"
	lessons := map[string]string{
		"engine.SpriteRenderer": "first-sprite", "animation.AnimationPlayer": "animation", "animation.Animator": "character",
		"particles.ParticleSystem2D": "explosion", "engine.MaterialOverride": "surface-channels", "engine.Light2D": "surface-channels", "engine.LightOccluder2D": "surface-channels",
	}
	for _, lang := range []string{"en", "ru"} {
		b := builder{lang: lang, opt: Options{StudioURL: base}}
		for id, lesson := range lessons {
			got := b.componentStudioLinks(&Entry{ID: id, Component: &ComponentInfo{}}, "../")
			for _, want := range []string{base + "examples/" + lesson + "/?lang=" + lang, base + "docs/" + lang + "/" + lesson + "/"} {
				if !strings.Contains(got, `href="`+want+`"`) {
					t.Errorf("%s %s missing pinned link %s: %s", lang, id, want, got)
				}
			}
			if strings.Contains(got, "<iframe") || strings.Contains(got, "<canvas") {
				t.Fatal("component page introduced a second editor surface")
			}
		}
		if lang == "ru" && !strings.Contains(b.componentStudioLinks(&Entry{ID: "engine.SpriteRenderer", Component: &ComponentInfo{}}, "../"), "Первый спрайт") {
			t.Fatal("Russian action label missing")
		}
	}
}

func TestComponentStudioLinksRespectAvailableSurfacesAndIntegratedGuides(t *testing.T) {
	e := &Entry{ID: "engine.SpriteRenderer", Component: &ComponentInfo{}}
	b := builder{}
	if b.componentStudioLinks(e, "../") != "" {
		t.Fatal("links emitted without an available Studio")
	}
	b.opt.Studio = true
	got := b.componentStudioLinks(e, "../")
	if !strings.Contains(got, `href="../studio/"`) || strings.Contains(got, "/examples/") || strings.Contains(got, "/docs/") {
		t.Fatal("legacy shell received nonexistent example routes", got)
	}
	b.opt = Options{StudioURL: "https://aeditor.akarpov.ru/r/checked/", StudioGuides: true}
	b.lang = "ru"
	got = b.componentStudioLinks(e, "../../")
	if !strings.Contains(got, `href="../../asset-studio/ru/first-sprite/"`) {
		t.Fatal("did not use integrated pinned guide", got)
	}
	if b.componentStudioLinks(&Entry{ID: "physics.Body2D", Component: &ComponentInfo{}}, "../") != "" {
		t.Fatal("unrelated component was given a graphic authoring link")
	}
	if b.componentStudioLinks(&Entry{ID: "engine.SpriteRenderer"}, "../") != "" {
		t.Fatal("noncomponent was treated as a component")
	}
}
