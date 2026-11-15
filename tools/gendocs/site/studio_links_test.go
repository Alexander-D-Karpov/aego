package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComponentStudioLinksAreLocalizedAndOptional(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range []string{"en", "ru"} {
		for _, lesson := range []string{"first-sprite", "export", "animation", "character", "explosion", "surface-channels"} {
			p := filepath.Join(dir, "asset-studio", lang, lesson)
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, "index.html"), []byte("guide"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, spec := range []struct{ id, lesson string }{
		{"engine.SpriteRenderer", "first-sprite"},
		{"animation.AnimationPlayer", "animation"},
		{"animation.Animator", "character"},
		{"particles.ParticleSystem2D", "explosion"},
		{"engine.MaterialOverride", "surface-channels"},
		{"engine.Light2D", "surface-channels"},
		{"engine.LightOccluder2D", "surface-channels"},
	} {
		for _, lang := range []string{"en", "ru"} {
			b := &builder{out: dir, lang: lang, opt: Options{StudioGuides: true}}
			entry := &Entry{ID: spec.id, Component: &ComponentInfo{}}
			got := b.componentStudioLinks(entry, "../../")
			want := `href="../../asset-studio/` + lang + `/` + spec.lesson + `/"`
			if !strings.Contains(got, want) {
				t.Errorf("%s %s: expected %s: %s", spec.id, lang, want, got)
			}
			if strings.Contains(got, "<iframe") || strings.Contains(got, "<script") {
				t.Errorf("%s %s eagerly loads Studio", spec.id, lang)
			}
			b.opt.StudioGuides = false
			if b.componentStudioLinks(entry, "../../") != "" {
				t.Errorf("%s %s linked absent optional integration", spec.id, lang)
			}
		}
	}
	if err := os.Remove(filepath.Join(dir, "asset-studio", "ru", "animation", "index.html")); err != nil {
		t.Fatal(err)
	}
	b := &builder{out: dir, lang: "ru", opt: Options{StudioGuides: true}}
	if got := b.componentStudioLinks(&Entry{ID: "animation.AnimationPlayer", Component: &ComponentInfo{}}, "../../"); got != "" {
		t.Fatalf("linked a missing lesson: %s", got)
	}
}

func TestComponentStudioLinksAppearOnRealGeneratedComponentPages(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range []string{"en", "ru"} {
		for _, lesson := range []string{"first-sprite", "export"} {
			p := filepath.Join(dir, "asset-studio", lang, lesson)
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(p, "index.html"), []byte("guide"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	exp := fixture()
	for i := range exp.Entries {
		if exp.Entries[i].ID == "engine.SpriteRenderer" {
			exp.Entries[i].Component = &ComponentInfo{}
		}
	}
	if _, err := Build(exp, dir, Options{StudioURL: "https://example.com/r/test/", StudioGuides: true}); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ page, path, text string }{
		{"ref/engine.SpriteRenderer.html", `href="../asset-studio/en/first-sprite/"`, "Draw your first sprite"},
		{"ru/ref/engine.SpriteRenderer.html", `href="../../asset-studio/ru/first-sprite/"`, "Первый спрайт"},
	} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(spec.page)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), spec.path) || !strings.Contains(string(data), spec.text) {
			t.Errorf("%s missing localized component link", spec.page)
		}
	}
}
