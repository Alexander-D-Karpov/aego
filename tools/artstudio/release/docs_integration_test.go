package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func docsArtifact(t *testing.T, guideOrigin string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	names := map[string]string{
		"docs.css":      "assets/docs.0123456789abcdef.css",
		"protocol.js":   "assets/protocol.0123456789abcdef.js",
		"docs-embed.js": "assets/docs-embed.0123456789abcdef.js",
	}
	for name, data := range map[string]string{
		"index.html": "home", "editor/index.html": "editor", "embed/index.html": "embed",
		"assets/core.0123456789abcdef.wasm": "wasm", names["docs.css"]: "css",
		names["protocol.js"]: "protocol", names["docs-embed.js"]: "embed",
		"examples/first-sprite/start.aeart": "art", "examples/first-sprite/preview.png": "png",
	} {
		if err := writeNew(filepath.Join(dir, filepath.FromSlash(name)), []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	const id = "test-build"
	if err := writeGuides(dir, names, nil, buildOptions{ID: id, Origin: guideOrigin}); err != nil {
		t.Fatal(err)
	}
	files, err := inventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := buildManifest{Schema: manifestVersion, DocumentVersion: 1, ProtocolVersion: embedProtocolVersion,
		BuildID: id, SourceSHA256: digest([]byte("source")), BuiltAt: "2026-11-07T00:00:00Z", Toolchain: "fixture",
		AppOrigin: "http://127.0.0.1:8097", DocsOrigins: []string{"http://127.0.0.1:8098"}, Files: files,
		Examples: []example{{ID: "first-sprite", Version: 1, Title: map[string]string{"en": "first", "ru": "первый"}, File: "examples/first-sprite/start.aeart", Thumbnail: "examples/first-sprite/preview.png", SHA256: digest([]byte("art")), Bytes: 3}},
	}
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".wasm") {
			m.WASM = f
		}
	}
	b := jsonBytes(m)
	if err := writeNew(filepath.Join(dir, "manifest.json"), b); err != nil {
		t.Fatal(err)
	}
	return dir, digest(b)
}

func TestIntegrateDocsPinsArtifactAndPreservesDestination(t *testing.T) {
	dir, sha := docsArtifact(t, "http://127.0.0.1:8097")
	site := t.TempDir()
	result, err := integrateDocs(dir, site, sha)
	if err != nil || result["guides"] != 2 || result["editorURL"] != "http://127.0.0.1:8097/r/test-build/" {
		t.Fatalf("integration: %+v %v", result, err)
	}
	for _, rel := range []string{"asset-studio/en/first-sprite/index.html", "asset-studio/ru/first-sprite/index.html", "asset-studio/integration.json", "assets/docs.0123456789abcdef.css", "assets/protocol.0123456789abcdef.js", "assets/docs-embed.0123456789abcdef.js"} {
		if _, err := os.Stat(filepath.Join(site, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(site, "assets/core.0123456789abcdef.wasm")); !os.IsNotExist(err) {
		t.Fatal("standalone WASM was copied into the documentation")
	}
	if _, err := integrateDocs(dir, site, sha); err != nil {
		t.Fatalf("identical repeated integration failed: %v", err)
	}
	conflict := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(filepath.Join(conflict, "asset-studio/ru/first-sprite"), 0o755); err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(conflict, "asset-studio/ru/first-sprite/index.html")
	if err := os.WriteFile(user, []byte("user content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := integrateDocs(dir, conflict, sha); err == nil {
		t.Fatal("conflicting site file was accepted")
	}
	if b, _ := os.ReadFile(user); string(b) != "user content" {
		t.Fatal("conflicting site content was changed")
	}
	if _, err := os.Stat(filepath.Join(conflict, "assets")); !os.IsNotExist(err) {
		t.Fatal("preflight conflict left partial copied assets")
	}
	if _, err := integrateDocs(dir, dir, sha); err == nil {
		t.Fatal("wrote documentation inside immutable artifact")
	}
}

func TestIntegrateDocsRejectsMismatchedAndUnsafeArtifact(t *testing.T) {
	dir, sha := docsArtifact(t, "http://127.0.0.1:8099")
	if _, err := integrateDocs(dir, t.TempDir(), sha); err == nil {
		t.Fatal("mismatched pinned origin was accepted")
	}
	dir, sha = docsArtifact(t, "http://127.0.0.1:8097")
	site := t.TempDir()
	if _, err := integrateDocs(dir, site, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong trusted manifest hash accepted")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(site, "asset-studio")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := integrateDocs(dir, site, sha); err == nil {
		t.Fatal("site symlink accepted")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("symlink target was modified")
	}
}
