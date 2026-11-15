package site

import (
	"strings"
	"testing"
)

func TestSchemaValuesStayOutOfTheComponentNavigation(t *testing.T) {
	exp := fixture()
	exp.Entries = append(exp.Entries, Entry{ID: "particles.ColorKey", Parent: "module/script", Kind: KindType, ValueType: true, Group: "components", Title: "ColorKey", Body: "A colour gradient key."})
	for i := range exp.Groups {
		if exp.Groups[i].ID == "components" {
			exp.Groups[i].Nav[0].Children = append(exp.Groups[i].Nav[0].Children, Node{ID: "particles.ColorKey"})
		}
	}
	dir, report, pages := build(t, exp, Options{})
	_ = dir
	page := pages["ref/particles.ColorKey.html"]
	if page == "" || !strings.Contains(page, "ColorKey") {
		t.Fatal("value structure lost its addressable reference")
	}
	nav := strings.Split(strings.Split(page, `<nav class="side"`)[1], "</nav>")[0]
	if strings.Contains(nav, "particles.ColorKey") || strings.Contains(nav, ">ColorKey<") {
		t.Fatal("value structure is listed as an addable component")
	}
	if strings.Contains(report.Outline, "ColorKey") {
		t.Fatal("navigation golden outline differs from the visible menu")
	}
	if strings.Contains(page, `class="docs-version"`) {
		t.Fatal("removed version badge returned")
	}
}

func TestUnlinkedStartGroupDoesNotRepeatItsSectionLabel(t *testing.T) {
	b := builder{own: map[string]bool{"install": true}, urls: map[string]string{"install": "install.html"},
		byID:   map[string]*Entry{"install": {ID: "install", Title: "Install Aego", Kind: KindPage}},
		groups: []navGroup{{Group: Group{ID: "start", Title: "Getting started", Section: SectionStart}, roots: []string{"install", "quickstart"}}}}
	// Two roots prevent head() from treating Install as the group label.
	b.byID["quickstart"] = &Entry{ID: "quickstart", Title: "Your first project", Kind: KindPage}
	b.own["quickstart"], b.urls["quickstart"] = true, "quickstart.html"
	if n := strings.Count(b.nav("", ""), "Getting started"); n != 1 {
		t.Fatalf("repeated start label: %d", n)
	}
}
