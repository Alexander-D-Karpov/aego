package site

import (
	"encoding/json"
	"flag"
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the navigation goldens")

var (
	hrefRe = regexp.MustCompile(`href="([^"]*)"`)
	idRe   = regexp.MustCompile(`id="([^"]*)"`)
)

func fixture() *Export {
	return &Export{
		Version: 2,
		Title:   "Aego",
		Entries: []Entry{
			{ID: "welcome", Kind: KindPage, Group: "start", Title: "Getting started", Summary: "First steps.",
				Body:     "# Getting started\n\n## Install\n\nGet Go.\n\n## Run\n\nRun it.",
				Location: Location{Path: "engine:welcome.md", Line: 1, Source: "engine/res/docs/welcome.md"}},
			{ID: "language", Kind: KindPage, Group: "script", Title: "Aego Script", Summary: "The language.", Body: "# Aego Script\n\nSee [coroutines](ref:language/coroutines)."},
			{ID: "language/coroutines", Parent: "language", Kind: KindPage, Group: "script", Title: "Coroutines", Body: "# Coroutines\n\nPause with [yield](ref:keyword/yield), clamp with `aego/math.Clamp`, fails with `AES2050`.\n\n```aes run\npackage demo\n```\n"},
			{ID: "language/builtins", Parent: "language", Kind: KindPage, Group: "script", Title: "Keywords", Body: "# Keywords\n\nAll of them."},
			{ID: "keyword/yield", Parent: "language/builtins", Kind: KindPage, Group: "script", Title: "yield", Body: "yield pauses.\n\n```aes\nyield wait(0.1)\n```"},
			{ID: "aego/math", Kind: KindModule, Group: "ref-script", Title: "aego/math"},
			{ID: "aego/math.Clamp", Parent: "aego/math", Kind: KindFunction, Group: "ref-script", Title: "Clamp", Signature: "func Clamp(x float, lo float, hi float) float", Body: "Clamp limits x.", Example: "math.Clamp(v, 0, 1)"},
			{ID: "module/script", Kind: KindModule, Group: "components", Title: "Aego Script"},
			{ID: "engine.SpriteRenderer", Parent: "module/script", Kind: KindType, Group: "components", Title: "Sprite Renderer", Body: "Draws a sprite."},
			{ID: "engine.SpriteRenderer.Tint", Parent: "engine.SpriteRenderer", Kind: KindField, Group: "components", Title: "Tint", Body: "Color."},
			{ID: "diagnostics", Kind: KindPage, Group: "diagnostics", Title: "Diagnostics", Body: "Every code."},
			{ID: "AES2050", Parent: "diagnostics", Kind: KindCode, Group: "diagnostics", Title: "AES2050", Body: "yield outside a coroutine.\n\nFix: use `coroutine`."},
			{ID: "project", Kind: KindPage, Group: "project", Title: "My game", Body: "Script packages."},
			{ID: "scripts/game/player", Parent: "project", Kind: KindModule, Group: "project", Title: "game/player", Location: Location{Path: "scripts/player/player.aes", Line: 3}},
		},
		Groups: []GroupNav{
			{Group: Group{ID: "ref-script", Title: "Script API", Section: SectionReference, Order: 100},
				Nav: []Node{{ID: "aego/math", Children: []Node{{ID: "aego/math.Clamp"}}}}},
			{Group: Group{ID: "start", Title: "Getting started", Section: SectionStart}, Nav: []Node{{ID: "welcome"}}},
			{Group: Group{ID: "script", Title: "Aego Script", Section: SectionGuides, Order: 20}, Nav: []Node{{ID: "language", Children: []Node{
				{ID: "language/coroutines"},
				{ID: "language/builtins", Children: []Node{{ID: "keyword/yield"}}},
			}}}},
			{Group: Group{ID: "components", Title: "Components", Section: SectionReference, Order: 110},
				Nav: []Node{{ID: "module/script", Children: []Node{{ID: "engine.SpriteRenderer", Children: []Node{{ID: "engine.SpriteRenderer.Tint"}}}}}}},
			{Group: Group{ID: "diagnostics", Title: "Diagnostics", Section: SectionReference, Order: 130}, Nav: []Node{{ID: "diagnostics", Children: []Node{{ID: "AES2050"}}}}},
			{Group: Group{ID: "project", Title: "Project", Section: SectionReference, Order: 150}, Nav: []Node{{ID: "project", Children: []Node{{ID: "scripts/game/player"}}}}},
		},
	}
}

func crawl(t *testing.T, dir string) map[string]string {
	t.Helper()
	pages := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		pages[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]map[string]bool{}
	for p, body := range pages {
		ids[p] = map[string]bool{}
		for _, m := range idRe.FindAllStringSubmatch(body, -1) {
			ids[p][html.UnescapeString(m[1])] = true
		}
	}
	for p, body := range pages {
		for _, m := range hrefRe.FindAllStringSubmatch(body, -1) {
			href := html.UnescapeString(m[1])
			if href == "#" {
				t.Errorf("%s: placeholder link", p)
				continue
			}
			if strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "data:") {
				continue
			}
			target, frag, _ := strings.Cut(href, "#")
			target, _, _ = strings.Cut(target, "?")
			file := p
			if target != "" {
				file = path.Clean(path.Join(path.Dir(p), target))
				if strings.HasSuffix(target, "/") {
					file = path.Join(file, "index.html")
				}
			}
			if strings.HasSuffix(file, ".css") || strings.HasSuffix(file, ".js") {
				if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
					t.Errorf("%s: missing asset %s", p, href)
				}
				continue
			}
			got, ok := ids[file]
			if !ok {
				t.Errorf("%s: dead link %s", p, href)
				continue
			}
			if frag != "" && !got[frag] {
				t.Errorf("%s: missing anchor %s", p, href)
			}
		}
	}
	return pages
}

func build(t *testing.T, exp *Export, opt Options) (string, Report, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	rep, err := Build(exp, dir, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range rep.Broken {
		t.Errorf("broken reference %s", b)
	}
	pages := crawl(t, dir)
	for _, e := range exp.Entries {
		if e.Kind == KindModule {
			if _, ok := pages[pagePath(&e)]; !ok {
				t.Errorf("module %s has no page", e.ID)
			}
		}
	}
	return dir, rep, pages
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	file := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%s: %v (run go test ./site -update and review it)", name, err)
	}
	if string(want) != got {
		t.Errorf("%s changed, run go test ./site -update and review the diff:\n%s", name, got)
	}
}

func TestFixtureSite(t *testing.T) {
	_, _, pages := build(t, fixture(), Options{})
	co := pages["language/coroutines.html"]
	for _, want := range []string{
		`href="../language/builtins.html#keyword/yield"`,
		`href="../ref/aego/math.html#aego/math.Clamp"`,
		`href="../diagnostics.html#AES2050"`,
		`<span class="t-keyword">package</span>`,
	} {
		if !strings.Contains(co, want) {
			t.Errorf("coroutines page has no %s", want)
		}
	}
	if !strings.Contains(pages["language/builtins.html"], `id="keyword/yield"`) {
		t.Error("keyword is not a section of the builtins page")
	}
	if strings.Contains(co, `class="snippet"`) {
		t.Error("run buttons without a playground")
	}
	if !strings.Contains(pages["ref/scripts/game/player.html"], "scripts/player/player.aes:3") {
		t.Error("project script page has no source location")
	}
}

func TestNavOutline(t *testing.T) {
	_, rep, _ := build(t, fixture(), Options{})
	if !strings.HasPrefix(rep.Outline, "Getting started\n") {
		t.Fatalf("sections are out of order:\n%s", rep.Outline)
	}
	golden(t, "nav.golden", rep.Outline)
}

func TestShell(t *testing.T) {
	_, _, pages := build(t, fixture(), Options{Repo: "https://git.example/aego", Version: "1.2.3", Commit: "abc1234"})
	w := pages["welcome.html"]
	for _, want := range []string{
		`href="https://git.example/aego/src/branch/main/engine/res/docs/welcome.md#L1"`,
		`href="https://git.example/aego/commit/abc1234"`,
		`Aego 1.2.3`,
		`<a href="#install">Install</a>`,
		`class="next" href="language.html"`,
		`data-group="start" open data-current="true"`,
		`aria-current="page"`,
	} {
		if !strings.Contains(w, want) {
			t.Errorf("welcome page has no %s", want)
		}
	}
	co := pages["language/coroutines.html"]
	if !strings.Contains(co, `<nav class="crumbs"><span>Guides</span><span class="sep">/</span><span>Aego Script</span><span class="sep">/</span><a href="../language.html">Aego Script</a></nav>`) {
		t.Error("breadcrumbs are wrong on the coroutines page")
	}
	if !strings.Contains(pages["index.html"], `class="button primary" href="welcome.html"`) {
		t.Error("landing page has no start button")
	}
}

func TestSearchAndLlms(t *testing.T) {
	dir, _, _ := build(t, fixture(), Options{})
	data, err := os.ReadFile(filepath.Join(dir, "search.json"))
	if err != nil {
		t.Fatal(err)
	}
	var items []searchItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Section == "" || it.Group == "" {
			t.Errorf("search item %s has no category", it.ID)
		}
	}
	llms, err := os.ReadFile(filepath.Join(dir, "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Guides", "### Aego Script", "- [Aego Script](language.html): The language."} {
		if !strings.Contains(string(llms), want) {
			t.Errorf("llms.txt has no %q", want)
		}
	}
}

func TestRunButtons(t *testing.T) {
	dir := t.TempDir()
	if _, err := Build(fixture(), dir, Options{Playground: true}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "language", "coroutines.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `data-example="language/coroutines/1"`) {
		t.Fatal("run block has no example id matching the playground")
	}
}

func TestRealExport(t *testing.T) {
	in := os.Getenv("AEGO_DOCS_EXPORT")
	if in == "" {
		t.Skip("AEGO_DOCS_EXPORT is not set")
	}
	exp, err := Load(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp.Groups) == 0 {
		t.Fatal("the export has no groups, rebuild the aego CLI")
	}
	_, rep, _ := build(t, exp, Options{})
	golden(t, "nav-real.golden", rep.Outline)
}
