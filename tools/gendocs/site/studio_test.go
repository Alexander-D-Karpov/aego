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
		{"ru/index.html", `href="../studio/"`, "Студия рисунка"},
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
