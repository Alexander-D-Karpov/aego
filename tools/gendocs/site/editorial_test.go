package site

import (
	"html"
	"regexp"
	"strings"
	"testing"
)

func TestLocalNamesAreNotRegistryLinks(t *testing.T) {
	conv := newConverter()
	source := "enum State { Idle, Run }\nstruct Weapon { Range float }\nfloor := node.Find(\"Floor\")\nvalues := []Weapon{Weapon{Range: 2}}\nother := node.Get<Weapon>()\n"
	md := "`Floor`, `State`, `Range` and `Weapon.Range` are user names.\n\n" +
		"[Explicit Floor](ref:Floor), `aego/math.Clamp`.\n\n```aes\n" + source + "```"
	aliases := map[string]string{"Floor": "wrong-floor", "State": "wrong-state", "Range": "wrong-range", "Weapon.Range": "wrong-field", "aego/math.Clamp": "math-clamp"}
	p := &pageCtx{resolve: func(id string) (string, bool) { u, ok := aliases[id]; return u, ok }, autoResolve: func(id string) (string, bool) { return "math-clamp", id == "aego/math.Clamp" }, broken: func(id string) { t.Errorf("broken: %s", id) }}
	got, err := conv.render(p, md)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, `href="wrong-floor"`) != 1 || strings.Contains(got, `href="wrong-state"`) || strings.Contains(got, `href="wrong-range"`) || strings.Contains(got, `href="wrong-field"`) {
		t.Fatalf("guessed a link for a local symbol: %s", got)
	}
	if !strings.Contains(got, `href="math-clamp"`) {
		t.Fatal("lost the qualified API reference")
	}
	code := regexp.MustCompile(`(?s)<code class="lang-aes">(.*?)</code>`).FindStringSubmatch(got)
	if len(code) != 2 || html.UnescapeString(tagRe.ReplaceAllString(code[1], "")) != source {
		t.Fatalf("copied code differs from source: %s", got)
	}
}

func TestEmbeddedArticleCannotAddAnotherH1(t *testing.T) {
	exp := fixture()
	for i := range exp.Entries {
		if exp.Entries[i].ID == "engine.SpriteRenderer" {
			exp.Entries[i].Component = &ComponentInfo{}
			exp.Entries[i].Body = "Schema description.\n\n# Embedded article\n\n## Details\n\nAn explanation."
			exp.Entries[i].Translations = map[string]Translation{"ru": {Body: "Описание.\n\n# Встроенная статья\n\n## Подробности\n\nПояснение."}}
		}
	}
	_, _, pages := build(t, exp, Options{})
	page := pages["ref/engine.SpriteRenderer.html"]
	if strings.Count(page, "<h1 ") != 1 || !strings.Contains(page, `<span class="legacy-anchor" id="embedded-article">`) || strings.Contains(page, ">Embedded article</h") || !strings.Contains(page, `<h2 id="details">`) {
		t.Fatalf("invalid embedded heading hierarchy: %s", page)
	}
	if translated := pages["ru/ref/engine.SpriteRenderer.html"]; !strings.Contains(translated, `<span class="legacy-anchor" id="embedded-article">`) || !strings.Contains(translated, `<h2 id="details">`) {
		t.Fatalf("translation lost the old embedded-title anchor: %s", translated)
	}
}
