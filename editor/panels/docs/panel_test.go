package docs

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"aego/app/docicons"
	"aego/app/docstool"
	"aego/core/mathx"
	"aego/editor/core/services"
	regdocs "aego/engine/docs"
	"aego/engine/i18n"
	"aego/engine/module"
	"aego/platform/input"
	"aego/ui/core"
)

func testRegistry() *regdocs.Registry {
	files := fstest.MapFS{
		"guide.md":        {Data: []byte("---\nid: quickstart\ngroup: start\norder: 1\n---\n# Quick start\n\nStart.\n\n---\nid: quickstart/child\nparent: quickstart\n---\n# First steps\n\nSteps.\n")},
		"guide.ru.md":     {Data: []byte("---\nid: quickstart\nsearch_names: [быстрый старт]\n---\n# Быстрый старт\n\nНачало.\n\n---\nid: quickstart/child\n---\n# Первые шаги\n\nШаги.\n")},
		"flight.md":       {Data: []byte("---\nid: flight\ngroup: flight\ngroup_title: Flight\ngroup_icon: cat.camera\ngroup_order: 45\nicon: file.prefab\norder: 1\n---\n# Flight\n\nFly.\n\n---\nid: flight/child\nparent: flight\n---\n# Wings\n\nFlap.\n")},
		"flight.ru.md":    {Data: []byte("---\nid: flight\ngroup: flight\ngroup_title: Полёт\n---\n# Полёт\n\nЛетать.\n")},
		"animation.md":    {Data: []byte("---\nid: animation\ngroup: flight\nicon: play.start\norder: 2\n---\n# Animation\n\nAnimate.\n")},
		"animation.ru.md": {Data: []byte("---\nid: animation\nsearch_names: [анимация]\n---\n# Анимация\n\nАнимировать.\n")},
	}
	r := regdocs.Assemble(regdocs.Sources{Docs: []module.DocSource{{Owner: "test", FS: files}}})
	r.SetNavigationIcons(docicons.Builtin())
	return r
}

func findRow(p *Panel, id string) (row, bool) {
	for _, r := range p.rows {
		if r.id == id {
			return r, true
		}
	}
	return row{}, false
}

func TestLocalizedNavigationAndSearchKeepIcons(t *testing.T) {
	bundle := i18n.Default()
	old := bundle.Language()
	defer bundle.SetLanguage(old)
	p := New(testRegistry(), nil, nil)
	p.Show("quickstart/child", "")
	p.open[groupKey("flight")] = true
	var order []string

	for _, tc := range []struct{ lang, groupTitle, rootTitle, childTitle string }{
		{"en", "Flight", "Quick start", "First steps"},
		{"ru", "Полёт", "Быстрый старт", "Первые шаги"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			bundle.SetLanguage(tc.lang)
			p.query = ""
			p.rebuild()
			var ids []string
			for _, rw := range p.rows {
				if !rw.head {
					ids = append(ids, rw.id)
				}
			}
			if order == nil {
				order = ids
			} else if !slices.Equal(ids, order) {
				t.Errorf("language changed navigation order: %v vs %v", ids, order)
			}
			for _, expected := range []struct{ id, title, icon string }{
				{groupKey("flight"), tc.groupTitle, "cat.camera"},
				{"quickstart", tc.rootTitle, "doc.project"},
				{"quickstart/child", tc.childTitle, "doc.project"},
				{"flight", map[string]string{"en": "Flight", "ru": "Полёт"}[tc.lang], "file.prefab"},
			} {
				got, ok := findRow(p, expected.id)
				if !ok || got.title != expected.title || got.icon != expected.icon {
					t.Errorf("row %q = %+v (%v), want title %q icon %q", expected.id, got, ok, expected.title, expected.icon)
				}
			}
			p.query = "quickstart"
			p.rebuild()
			hits := p.reg.Search(p.query, 200)
			if len(p.rows) != len(hits) {
				t.Fatalf("search row count = %d, want %d", len(p.rows), len(hits))
			}
			for i, hit := range hits {
				if p.rows[i].id != hit.ID {
					t.Errorf("search order changed at %d: %q, want %q", i, p.rows[i].id, hit.ID)
				}
			}
			got, ok := findRow(p, "quickstart/child")
			if !ok || got.title != tc.childTitle+"  ·  page" || got.icon != "doc.project" {
				t.Errorf("search row = %+v (%v)", got, ok)
			}
			if p.Current() != "quickstart/child" {
				t.Errorf("rebuild changed the current page: %s", p.Current())
			}
			if tc.lang == "ru" {
				p.query = "быстрый старт"
				p.rebuild()
				if got, ok := findRow(p, "quickstart"); !ok || got.icon != "doc.project" {
					t.Errorf("localized search row = %+v (%v)", got, ok)
				}
			}
		})
	}
}

func TestModuleIconPolicyInLocalizedTreeAndSearch(t *testing.T) {
	bundle := i18n.Default()
	old := bundle.Language()
	defer bundle.SetLanguage(old)
	p := New(testRegistry(), nil, nil)
	p.Show("animation", "")
	for _, tc := range []struct{ lang, title, query string }{
		{"en", "Animation", "animation"},
		{"ru", "Анимация", "анимация"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			bundle.SetLanguage(tc.lang)
			p.query = ""
			p.rebuild()
			rw, ok := findRow(p, "animation")
			if !ok || rw.title != tc.title || rw.icon != "animation.player" {
				t.Fatalf("localized navigation row = %+v (%v)", rw, ok)
			}
			p.query = tc.query
			p.rebuild()
			rw, ok = findRow(p, "animation")
			if !ok || rw.title != tc.title+"  ·  page" || rw.icon != "animation.player" || p.Current() != "animation" {
				t.Fatalf("localized search row = %+v (%v), current %q", rw, ok, p.Current())
			}
		})
	}
}

// Exercise the actual composed registry through the panel, not just a fixture
// policy: navigation, search and language changes must keep module icons.
func TestComposedRegistryModuleIconsInNavigationAndSearch(t *testing.T) {
	reg, err := docstool.Registry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bundle := i18n.Default()
	old := bundle.Language()
	defer bundle.SetLanguage(old)
	p := New(reg, nil, nil)
	set := embeddedIcons(t)
	for _, lang := range []string{"en", "ru"} {
		t.Run(lang, func(t *testing.T) {
			bundle.SetLanguage(lang)
			for _, tc := range []struct{ id, icon string }{
				{"animation", "animation.player"},
				{"animation/animator", "animation.animator"},
				{"render2d.PostProcess", "view.overlays"},
				{"camera.CameraBrain2D", "view.split"},
				{"aego/audio", "cat.audio"},
				{"aego/timer", "code.timer"},
				{"section/animation", "animation.player"},
				{"platformer-effects", "render2d.point_light"},
			} {
				p.Show(tc.id, "")
				if p.Current() != tc.id {
					t.Errorf("Show(%q) selected %q", tc.id, p.Current())
					continue
				}
				localized, ok := p.get(tc.id)
				if !ok {
					t.Fatalf("missing composed entry %q", tc.id)
				}
				p.query = ""
				p.rebuild()
				rw, ok := findRow(p, tc.id)
				if !ok || rw.title != localized.Title || rw.icon != tc.icon {
					t.Errorf("%s tree row %q = %+v (%v)", lang, tc.id, rw, ok)
				}
				p.query = tc.id
				p.rebuild()
				rw, ok = findRow(p, tc.id)
				if !ok || rw.title != localized.Title+"  ·  "+localized.Kind.String() || rw.icon != tc.icon {
					t.Errorf("%s search row %q = %+v (%v)", lang, tc.id, rw, ok)
				}
				if tc.id == "animation" {
					want := map[string]string{"en": "Animation", "ru": "Анимация"}[lang]
					if localized.Title != want {
						t.Errorf("%s animation title = %q, want %q", lang, localized.Title, want)
					}
				}
				if tc.id == "aego/timer" && !set.Has(tc.icon) && availableIcon(rw.icon, set.Has) != "play.speed" {
					t.Errorf("%s timer icon did not use its baked fallback", lang)
				}
			}
		})
	}
}

func TestPanelInputSearchRevealAndLanguageSwitch(t *testing.T) {
	bundle := i18n.Default()
	old := bundle.Language()
	defer bundle.SetLanguage(old)
	bundle.SetLanguage("en")
	p := New(testRegistry(), nil, nil)
	c := core.New(nil)
	c.Icons = embeddedIcons(t)
	in := &input.State{}
	full := mathx.Rect{W: 800, H: 200}
	frame := func(apply func()) {
		in.BeginFrame()
		if apply != nil {
			apply()
		}
		c.Begin(in, mathx.Vec2{X: full.W, Y: full.H}, 1)
		p.Draw(&services.Frame{UI: c}, full)
		c.End()
	}
	// A distant current row is still opened and scrolled into view.
	codes := p.reg.Children(regdocs.DiagnosticsID)
	if len(codes) == 0 {
		t.Fatal("fixture has no diagnostics")
	}
	last := codes[len(codes)-1].ID
	p.Show(last, "")
	frame(nil)
	if p.Current() != last || p.scroll <= 0 || !p.open[groupKey(regdocs.GroupDiagnostics)] || !p.open[regdocs.DiagnosticsID] {
		t.Fatalf("current page not revealed: %q scroll=%g open=%v", p.Current(), p.scroll, p.open)
	}
	beforeScroll := p.scroll
	bundle.SetLanguage("ru")
	frame(nil)
	if p.Current() != last || p.scroll != beforeScroll {
		t.Fatalf("language switch changed active row or tree scroll: %q %g (was %g)", p.Current(), p.scroll, beforeScroll)
	}
	bundle.SetLanguage("en")
	frame(nil)

	// Existing keyboard text input and result selection remain usable with icons.
	at := mathx.Vec2{X: 20, Y: 10}
	frame(func() { in.ApplyMouseDown(at, input.MouseLeft) })
	frame(func() { in.ApplyMouseUp(at, input.MouseLeft) })
	frame(func() { in.ApplyText([]byte("quickstart")) })
	if p.query != "quickstart" || c.Focus() == 0 {
		t.Fatalf("keyboard search lost focus or input: query=%q focus=%v", p.query, c.Focus())
	}
	rw, ok := findRow(p, "quickstart")
	if !ok || rw.icon != "doc.project" {
		t.Fatalf("search result missing icon: %+v", rw)
	}
	idx := -1
	for i, r := range p.rows {
		if r.id == "quickstart" {
			idx = i
			break
		}
	}
	y := c.Theme.RowHeight + 4 + c.Theme.Spacing + (float32(idx)+0.5)*c.Theme.RowHeight - p.scroll
	item := mathx.Vec2{X: 100, Y: y}
	frame(func() { in.ApplyMouseDown(item, input.MouseLeft) })
	frame(func() { in.ApplyMouseUp(item, input.MouseLeft) })
	if p.Current() != "quickstart" {
		t.Fatalf("search result click selected %q", p.Current())
	}

	p.text.Scroll = 37
	bundle.SetLanguage("ru")
	frame(nil)
	rw, ok = findRow(p, "quickstart")
	if !ok || !strings.HasPrefix(rw.title, "Быстрый старт") || rw.icon != "doc.project" || p.Current() != "quickstart" || p.text.Scroll != 37 {
		t.Fatalf("language switch lost selection, scroll or icon: row=%+v current=%q scroll=%g", rw, p.Current(), p.text.Scroll)
	}
}

func TestGroupIconDoesNotChangeRowToggle(t *testing.T) {
	p := New(testRegistry(), nil, nil)
	p.Show("flight", "")
	c := core.New(nil)
	c.Icons = embeddedIcons(t)
	in := &input.State{}
	full := mathx.Rect{W: 800, H: 600}
	frame := func(apply func()) {
		in.BeginFrame()
		if apply != nil {
			apply()
		}
		c.Begin(in, mathx.Vec2{X: full.W, Y: full.H}, 1)
		p.Draw(&services.Frame{UI: c}, full)
		c.End()
	}
	frame(nil)
	idx := -1
	for i, rw := range p.rows {
		if rw.id == groupKey("flight") {
			idx = i
			break
		}
	}
	if idx < 0 || !p.open[groupKey("flight")] {
		t.Fatal("current group was not opened")
	}
	// Hit the icon/label, not the disclosure arrow.
	y := c.Theme.RowHeight + 4 + c.Theme.Spacing + (float32(idx)+0.5)*c.Theme.RowHeight - p.scroll
	at := mathx.Vec2{X: c.Theme.Indent + 10, Y: y}
	frame(func() { in.ApplyMouseDown(at, input.MouseLeft) })
	frame(func() { in.ApplyMouseUp(at, input.MouseLeft) })
	if p.open[groupKey("flight")] || p.Current() != "flight" {
		t.Fatalf("group click changed selection or failed to toggle: open=%v current=%q", p.open[groupKey("flight")], p.Current())
	}
}
