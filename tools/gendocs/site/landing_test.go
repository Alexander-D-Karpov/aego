package site

import (
	"strings"
	"testing"
)

func TestLandingInsideCoversNewFeaturesInBothLanguages(t *testing.T) {
	_, _, pages := build(t, fixture(), Options{})
	for _, test := range []struct {
		page string
		text landingText
	}{
		{"index.html", landingEN},
		{"ru/index.html", landingRU},
	} {
		t.Run(test.text.lang, func(t *testing.T) {
			page := pages[test.page]
			_, section, ok := strings.Cut(page, `<section class="inside">`)
			if !ok {
				t.Fatal("home page lost What's inside")
			}
			section, _, ok = strings.Cut(section, "</section>")
			if !ok || !strings.Contains(section, `<h2 id="inside">`+esc(test.text.insideTitle)+`</h2>`) {
				t.Fatal("home page lost its localized heading or existing anchor")
			}
			if len(test.text.cards) != 14 || strings.Count(section, "<h3>") != len(test.text.cards) {
				t.Fatal("the expanded feature overview is incomplete")
			}
			seen := map[string]bool{}
			for i, card := range test.text.cards {
				if strings.TrimSpace(card[0]) == "" || strings.TrimSpace(card[1]) == "" || seen[card[0]] {
					t.Fatalf("empty or duplicate feature card at %d: %q", i, card[0])
				}
				seen[card[0]] = true
				if landingCardIcons[i] == "" || !strings.Contains(section, "<h3>"+esc(card[0])+"</h3><p>"+inlineMD(card[1])+"</p>") {
					t.Errorf("feature card %q is missing its icon or complete localized text", card[0])
				}
			}
			for _, title := range []string{"Art Studio", "Aego Script"} {
				if !seen[title] {
					t.Errorf("missing product feature %q", title)
				}
			}
		})
	}
}
