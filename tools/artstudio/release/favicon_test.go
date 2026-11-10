package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStudioAndGuidePagesHaveLocalFavicon(t *testing.T) {
	icon := regexp.MustCompile(`<link rel="icon"[^>]*href="([^"]+)"`)
	shell, err := os.ReadFile(filepath.Join("..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, contents []byte) string {
		t.Helper()
		match := icon.FindSubmatch(contents)
		if len(match) != 2 || !strings.HasPrefix(string(match[1]), "data:image/svg+xml,") {
			t.Fatalf("%s has no local SVG favicon", name)
		}
		return string(match[1])
	}
	want := check("development shell", shell)
	out := t.TempDir()
	names := map[string]string{
		"studio.css": "assets/studio.css", "surface.css": "assets/surface.css",
		"docs.css": "assets/docs.css", "config.js": "assets/config.js", "protocol.js": "assets/protocol.js",
		"docs-embed.js": "assets/docs-embed.js", "wasm_exec.js": "assets/wasm_exec.js",
	}
	if err := writeEditorPage(out, "editor", string(shell), names, "test"); err != nil {
		t.Fatal(err)
	}
	if err := writeLanding(out, names, nil, nil, "test", true); err != nil {
		t.Fatal(err)
	}
	if err := writeGuides(out, names, nil, buildOptions{ID: "test", Origin: "http://127.0.0.1:8097"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"index.html", "editor/index.html", "docs/en/first-sprite/index.html", "docs/ru/first-sprite/index.html", "docs/en/guide/getting-started/index.html", "docs/ru/guide/getting-started/index.html"} {
		data, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if got := check(path, data); got != want {
			t.Fatalf("%s has a different favicon", path)
		}
	}
}
