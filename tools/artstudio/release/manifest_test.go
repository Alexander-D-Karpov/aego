package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureArtifact(t *testing.T, id string) (string, buildManifest, string) {
	t.Helper()
	dir := filepath.Join(physicalTempDir(t), "artifact")
	if e := os.Mkdir(dir, 0755); e != nil {
		t.Fatal(e)
	}
	for p, b := range map[string][]byte{"index.html": []byte("home"), "editor/index.html": []byte("editor"), "embed/index.html": []byte("embed"), "assets/core.0123456789abcdef.wasm": []byte("\x00asm\x01\x00\x00\x00"), "assets/ui.0123456789abcdef.js": []byte("'use strict';"), "docs/en/first-sprite/index.html": []byte("guide")} {
		if e := writeNew(filepath.Join(dir, filepath.FromSlash(p)), b); e != nil {
			t.Fatal(e)
		}
	}
	files, e := inventory(dir)
	if e != nil {
		t.Fatal(e)
	}
	m := buildManifest{Schema: 1, AegoVersion: "0.12.1", StudioVersion: "test", DocumentVersion: 1, ProtocolVersion: 1, BuildID: id, SourceSHA256: digest([]byte("source")), BuiltAt: "2026-11-07T00:00:00Z", Toolchain: "fixture", AppOrigin: "http://127.0.0.1:8097", DocsOrigins: []string{"http://127.0.0.1:8098"}, Files: files}
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".wasm") {
			m.WASM = f
		}
	}
	b := jsonBytes(m)
	if e = writeNew(filepath.Join(dir, "manifest.json"), b); e != nil {
		t.Fatal(e)
	}
	return dir, m, digest(b)
}
func TestManifestIntegrity(t *testing.T) {
	dir, _, hash := fixtureArtifact(t, "one")
	if _, _, e := verify(dir, hash); e != nil {
		t.Fatal(e)
	}
	if _, _, e := verify(dir, strings.Repeat("0", 64)); e == nil {
		t.Fatal("accepted wrong expected SHA")
	}
	if e := os.WriteFile(filepath.Join(dir, "index.html"), []byte("tampered"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, _, e := verify(dir, hash); e == nil {
		t.Fatal("tampered file accepted")
	}
}
func TestManifestRejectsExtraMissingAndSymlink(t *testing.T) {
	for _, kind := range []string{"extra", "missing", "symlink", "parent-link", "ancestor-link"} {
		t.Run(kind, func(t *testing.T) {
			dir, _, h := fixtureArtifact(t, "one")
			if _, _, err := verify(dir, h); err != nil {
				t.Fatalf("valid fixture rejected before mutation: %v", err)
			}
			var wantError string
			switch kind {
			case "extra":
				if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("not published"), 0644); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(filepath.Join(dir, "index.html")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				link := filepath.Join(dir, "index.html")
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				fixtureSymlink(t, "editor/index.html", link)
				wantError = "symlink refused: " + link
			case "parent-link", "ancestor-link":
				link := filepath.Join(physicalTempDir(t), "link")
				target := dir
				if kind == "ancestor-link" {
					target = filepath.Dir(dir)
				}
				fixtureSymlink(t, target, link)
				dir = link
				if kind == "ancestor-link" {
					dir = filepath.Join(link, "artifact")
				}
				wantError = "symlink path refused: " + link
			}
			if _, _, e := verify(dir, h); e == nil {
				t.Fatal("invalid artifact accepted")
			} else if wantError != "" && e.Error() != wantError {
				t.Fatalf("verification error = %v; want %s", e, wantError)
			}
		})
	}
}
func TestManifestValidation(t *testing.T) {
	_, original, _ := fixtureArtifact(t, "one")
	cases := map[string]func(*buildManifest){
		"path traversal":     func(m *buildManifest) { m.Files[0].Path = "../escape" },
		"absolute":           func(m *buildManifest) { m.Files[0].Path = "/root/secret" },
		"backslash":          func(m *buildManifest) { m.Files[0].Path = `a\b` },
		"encoded traversal":  func(m *buildManifest) { m.Files[0].Path = "%2e%2e/x" },
		"duplicate":          func(m *buildManifest) { m.Files = append(m.Files, m.Files[0]) },
		"negative size":      func(m *buildManifest) { m.Files[0].Bytes = -1 },
		"oversized":          func(m *buildManifest) { m.Files[0].Bytes = maxFileBytes + 1 },
		"wrong mime":         func(m *buildManifest) { m.Files[0].MIME = "text/html" },
		"new protocol":       func(m *buildManifest) { m.ProtocolVersion = 2 },
		"new document":       func(m *buildManifest) { m.DocumentVersion = 2 },
		"bad origin":         func(m *buildManifest) { m.DocsOrigins = []string{"https://aego.akarpov.ru/evil"} },
		"origin credentials": func(m *buildManifest) { m.AppOrigin = "https://user:pass@example.com" },
		"fake release":       func(m *buildManifest) { m.Release = true },
		"wasm mismatch":      func(m *buildManifest) { m.WASM.SHA256 = strings.Repeat("0", 64) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var m buildManifest
			_ = json.Unmarshal(jsonBytes(original), &m)
			change(&m)
			if _, e := decodeManifest(jsonBytes(m)); e == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}
func TestSourceOrigins(t *testing.T) {
	for _, s := range []string{"*", "null", "javascript:alert(1)", "https://aego.akarpov.ru/", "https://evil@aego.akarpov.ru", "http://192.168.1.2", "https://aego.akarpov.ru#x"} {
		if validOrigin(s, true) {
			t.Fatalf("accepted %q", s)
		}
	}
	if !validOrigin("https://aego.akarpov.ru", true) || !validOrigin("http://127.0.0.1:8098", false) || validOrigin("http://127.0.0.1:8098", true) {
		t.Fatal("origin policy")
	}
}
func TestExamplesUseCoreAndHaveBothLocales(t *testing.T) {
	out := t.TempDir()
	examples, e := writeExamples(out, "examples")
	if e != nil {
		t.Fatal(e)
	}
	if len(examples) != 12 {
		t.Fatal("need twelve lesson documents")
	}
	for _, v := range examples {
		if v.Title["en"] == "" || v.Title["ru"] == "" {
			t.Fatal(v.ID)
		}
		b, e := os.ReadFile(filepath.Join(out, v.File))
		if e != nil || digest(b) != v.SHA256 {
			t.Fatal(v.ID)
		}
	}
	out2 := t.TempDir()
	other, e := writeExamples(out2, "examples")
	if e != nil {
		t.Fatal(e)
	}
	if string(jsonBytes(examples)) != string(jsonBytes(other)) {
		t.Fatal("examples are not deterministic")
	}
}
