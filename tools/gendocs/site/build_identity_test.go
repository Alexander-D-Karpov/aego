package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepresentationsShareOneDocumentationBuild(t *testing.T) {
	dir, _, pages := build(t, fixture(), Options{Version: "0.12.1", Commit: "example-dirty", Channel: "development"})
	data, err := os.ReadFile(filepath.Join(dir, "docs-build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ BuildID, RegistrySHA256 string }
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.BuildID) != 64 {
		t.Fatal("missing build digest")
	}
	for path, page := range pages {
		if !strings.Contains(page, `name="aego-docs-build" content="`+manifest.BuildID+`"`) {
			t.Errorf("%s: wrong HTML build", path)
		}
	}
	for _, path := range []string{"welcome.md", "ru/welcome.md", "llms.txt", "ru/llms.txt", "llms-full.txt", "ru/llms-full.txt", "search.json", "ru/search.json"} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), manifest.BuildID) {
			t.Errorf("%s: wrong build", path)
		}
	}
	registry, err := os.ReadFile(filepath.Join(dir, "docs-registry.json"))
	if err != nil || digest(registry) != manifest.RegistrySHA256 {
		t.Fatal("published registry differs from rendered snapshot", err)
	}
}
