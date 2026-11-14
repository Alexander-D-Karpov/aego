package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStudioIsOptionalLazyAndMustExist(t *testing.T) {
	dir := t.TempDir()
	if _, err := Build(fixture(), dir, Options{Studio: true}); err == nil {
		t.Fatal("a dead Studio link was generated")
	}
	if err := os.Mkdir(filepath.Join(dir, "studio"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "studio", "index.html"), []byte("<!doctype html><title>Studio</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(fixture(), dir, Options{Studio: true}); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ page, link, label string }{
		{"index.html", `href="studio/"`, "Art Studio"},
		{"ru/index.html", `href="../studio/"`, "Art Studio"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(spec.page)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), spec.link) || !strings.Contains(string(b), spec.label) {
			t.Fatalf("%s has no localized studio link", spec.page)
		}
		for _, eager := range []string{"artstudio.wasm", "studio.js", "wasm_exec.js"} {
			if strings.Contains(string(b), eager) {
				t.Fatalf("%s eagerly includes %s", spec.page, eager)
			}
		}
	}
}

func TestPinnedStudioLinksUseIntegratedGuidesWithoutLocalWASM(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range []string{"en", "ru"} {
		path := filepath.Join(dir, "asset-studio", lang, "first-sprite")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "index.html"), []byte("guide"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const pinned = "https://aeditor.akarpov.ru/r/build-1/"
	if _, err := Build(fixture(), dir, Options{StudioURL: pinned, StudioGuides: true}); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []struct{ page, guide, label, action string }{
		{"index.html", `href="asset-studio/en/first-sprite/"`, "Art Studio lessons", "Open Art Studio"},
		{"ru/index.html", `href="../asset-studio/ru/first-sprite/"`, "Уроки Art Studio", "Открыть Art Studio"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(spec.page)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `href="`+pinned+`editor/"`) || !strings.Contains(string(b), spec.guide) || !strings.Contains(string(b), spec.label) {
			t.Fatalf("%s does not link pinned editor and localized guide", spec.page)
		}
		if !strings.Contains(string(b), `<a class="button" href="`+pinned+`editor/">`+spec.action+`</a>`) {
			t.Fatalf("%s does not show a localized Studio launch button", spec.page)
		}
		if strings.Contains(string(b), `href="studio/"`) || strings.Contains(string(b), `href="../studio/"`) {
			t.Fatalf("%s links to the legacy local Studio", spec.page)
		}
		for _, eager := range []string{"artstudio.wasm", "docs-embed.", "protocol."} {
			if strings.Contains(string(b), eager) {
				t.Fatalf("%s eagerly includes %s", spec.page, eager)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "studio")); !os.IsNotExist(err) {
		t.Fatal("pinned integration unexpectedly created a second local Studio")
	}
}

func TestPinnedStudioRejectsInvalidURLsAndMissingGuides(t *testing.T) {
	for _, bad := range []string{"javascript:alert(1)", "http://example.com/r/build/", "https://user@example.com/r/build/", "https://example.com/editor/", "https://example.com/r/build/?x=1", "https://example.com/r/%2e%2e/"} {
		if _, err := Build(fixture(), t.TempDir(), Options{StudioURL: bad}); err == nil {
			t.Fatalf("accepted unsafe or unpinned URL %q", bad)
		}
	}
	if _, err := Build(fixture(), t.TempDir(), Options{StudioURL: "https://example.com/r/build/", StudioGuides: true}); err == nil {
		t.Fatal("linked missing guides")
	}
}
