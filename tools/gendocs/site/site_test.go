package site

import (
	"html"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	hrefRe = regexp.MustCompile(`href="([^"]*)"`)
	idRe   = regexp.MustCompile(`id="([^"]*)"`)
)

func fixture() *Export {
	return &Export{
		Title: "Aego",
		Entries: []Entry{
			{ID: "language", Kind: KindPage, Title: "Aego Script", Summary: "The language.", Body: "# Aego Script\n\nSee [coroutines](ref:language/coroutines)."},
			{ID: "language/coroutines", Parent: "language", Kind: KindPage, Title: "Coroutines", Body: "# Coroutines\n\nPause with [yield](ref:keyword/yield), clamp with `aego/math.Clamp`, fails with `AES2050`.\n\n```aes run\npackage demo\n```\n"},
			{ID: "language/builtins", Parent: "language", Kind: KindPage, Title: "Keywords", Body: "# Keywords\n\nAll of them."},
			{ID: "keyword/yield", Parent: "language/builtins", Kind: KindPage, Title: "yield", Body: "yield pauses.\n\n```aes\nyield wait(0.1)\n```"},
			{ID: "module/script", Kind: KindModule, Title: "script"},
			{ID: "aego/math", Parent: "module/script", Kind: KindModule, Title: "aego/math"},
			{ID: "aego/math.Clamp", Parent: "aego/math", Kind: KindFunction, Title: "Clamp", Signature: "func Clamp(x float, lo float, hi float) float", Body: "Clamp limits x.", Example: "math.Clamp(v, 0, 1)"},
			{ID: "engine.SpriteRenderer", Parent: "module/script", Kind: KindType, Title: "Sprite Renderer", Body: "Draws a sprite."},
			{ID: "engine.SpriteRenderer.Tint", Parent: "engine.SpriteRenderer", Kind: KindField, Title: "Tint", Body: "Color."},
			{ID: "diagnostics", Kind: KindPage, Title: "Diagnostics"},
			{ID: "AES2050", Parent: "diagnostics", Kind: KindCode, Title: "AES2050", Body: "yield outside a coroutine.\n\nFix: use `coroutine`."},
			{ID: "scripts/game/player", Parent: "project", Kind: KindModule, Title: "game/player", Location: Location{Path: "scripts/player/player.aes", Line: 3}},
			{ID: "project", Kind: KindPage, Title: "My game", Body: "Script packages."},
		},
		Nav: []Node{
			{ID: "language", Children: []Node{
				{ID: "language/coroutines"},
				{ID: "language/builtins", Children: []Node{{ID: "keyword/yield"}}},
			}},
			{ID: "module/script", Children: []Node{
				{ID: "aego/math", Children: []Node{{ID: "aego/math.Clamp"}}},
				{ID: "engine.SpriteRenderer", Children: []Node{{ID: "engine.SpriteRenderer.Tint"}}},
			}},
			{ID: "diagnostics", Children: []Node{{ID: "AES2050"}}},
			{ID: "project", Children: []Node{{ID: "scripts/game/player"}}},
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
			if href == "#" || strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
				if href == "#" {
					t.Errorf("%s: placeholder link", p)
				}
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

func check(t *testing.T, exp *Export) map[string]string {
	t.Helper()
	dir := t.TempDir()
	rep, err := Build(exp, dir, Options{})
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
	return pages
}

func TestFixtureSite(t *testing.T) {
	pages := check(t, fixture())
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
	check(t, exp)
}
