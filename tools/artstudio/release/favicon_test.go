package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"aego/app/brand"
)

func writeTestStudioIcon(t *testing.T, out string, names map[string]string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "web", "aego.png"))
	if err != nil {
		t.Fatal(err)
	}
	canonical, ok := brand.PNG(32)
	if !ok || !bytes.Equal(b, canonical) {
		t.Fatal("Studio icon differs from the Aego brand icon")
	}
	names["aego.png"], err = asset(out, "aego.png", b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStudioAndGuidePagesHaveLocalFavicon(t *testing.T) {
	icon := regexp.MustCompile(`<link rel="icon"[^>]*href="([^"]+)"`)
	shell, err := os.ReadFile(filepath.Join("..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	names := map[string]string{
		"studio.css": "assets/studio.css", "surface.css": "assets/surface.css",
		"docs.css": "assets/docs.css", "config.js": "assets/config.js", "protocol.js": "assets/protocol.js",
		"docs-embed.js": "assets/docs-embed.js", "wasm_exec.js": "assets/wasm_exec.js",
	}
	want := writeTestStudioIcon(t, out, names)
	if match := icon.FindSubmatch(shell); len(match) != 2 || string(match[1]) != "aego.png" {
		t.Fatal("development shell must use the local Aego favicon")
	}
	if !strings.Contains(string(shell), `<img class="brand-mark" src="aego.png" alt="">`) {
		t.Fatal("development shell must display the Aego icon")
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
		match := icon.FindSubmatch(data)
		if len(match) != 2 {
			t.Fatalf("%s has no favicon", path)
		}
		url := string(match[1])
		if path == "editor/index.html" {
			if url != "/r/test/"+names["aego.png"] || !strings.Contains(string(data), `<img class="brand-mark" src="`+url+`" alt="">`) {
				t.Fatal("editor does not use the versioned Aego icon")
			}
			continue
		}
		encoded, ok := strings.CutPrefix(url, "data:image/png;base64,")
		got, err := base64.StdEncoding.DecodeString(encoded)
		if !ok || err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s has a different favicon", path)
		}
	}
}
