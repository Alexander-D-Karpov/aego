package docstool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestStudioWebPackageVersionsEveryBridgeResource(t *testing.T) {
	web := filepath.Join("..", "..", "tools", "artstudio", "web")
	dir := t.TempDir()
	wasm := filepath.Join(dir, "artstudio.wasm")
	if err := os.WriteFile(wasm, []byte("wasm-build-one"), 0o644); err != nil {
		t.Fatal(err)
	}
	support := []byte("versioned go wasm support")
	check := func() string {
		t.Helper()
		if err := packageStudioWeb(web, dir, support); err != nil {
			t.Fatal(err)
		}
		index, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		js, err := os.ReadFile(filepath.Join(dir, "studio.js"))
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`studio\.css\?v=([a-f0-9]{10})`).FindSubmatch(index)
		if len(match) != 2 {
			t.Fatalf("no versioned stylesheet: %s", index)
		}
		version := string(match[1])
		for _, item := range []string{"wasm_exec.js", "recovery.js", "studio.js"} {
			if !strings.Contains(string(index), item+"?v="+version) {
				t.Fatalf("%s is not bound to %s", item, version)
			}
		}
		if !strings.Contains(string(js), "fetch('artstudio.wasm?v="+version+"')") {
			t.Fatal("WASM fetch is not bound to the same build")
		}
		actual, err := os.ReadFile(filepath.Join(dir, "wasm_exec.js"))
		if err != nil || string(actual) != string(support) {
			t.Fatal("WASM support changed")
		}
		return version
	}
	first := check()
	if second := check(); first != second {
		t.Fatalf("same inputs are not deterministic: %s != %s", first, second)
	}
	if err := os.WriteFile(wasm, []byte("wasm-build-two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if next := check(); next == first {
		t.Fatal("changed WASM reused the old bridge version")
	}
}

func TestStudioWebPackageRejectsUnknownEntryPoints(t *testing.T) {
	web, dir := t.TempDir(), t.TempDir()
	for name, contents := range map[string]string{"index.html": "unexpected index", "studio.css": "css", "studio.js": "fetch('artstudio.wasm')", "recovery.js": "js"} {
		if err := os.WriteFile(filepath.Join(web, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "artstudio.wasm"), []byte("wasm"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := packageStudioWeb(web, dir, []byte("support")); err == nil {
		t.Fatal("unknown index was silently packaged")
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); !os.IsNotExist(err) {
		t.Fatalf("invalid index was published: %v", err)
	}
}
