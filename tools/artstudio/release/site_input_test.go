package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionSiteInputRequiresPinnedRelease(t *testing.T) {
	const app = "https://aeditor.akarpov.ru"
	const docs = "https://aego.akarpov.ru"
	commit := strings.Repeat("a", 40)
	root := t.TempDir()
	for _, name := range []string{"art", "tools/artstudio"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "art", "input.go"), []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	source, err := sourceHash(root)
	if err != nil {
		t.Fatal(err)
	}

	type request struct {
		sha, commit, app, docs string
		change                 func(string, *buildManifest)
	}
	tests := map[string]request{
		"valid":             {},
		"missing hash":      {sha: "missing"},
		"wrong hash":        {sha: strings.Repeat("0", 64)},
		"short commit":      {commit: "abcd"},
		"wrong commit":      {commit: strings.Repeat("b", 40)},
		"wrong app origin":  {app: "https://unexpected.example"},
		"wrong docs origin": {docs: "https://unexpected.example"},
		"snapshot":          {change: func(_ string, m *buildManifest) { m.Release = false }},
		"extra parent origin": {change: func(_ string, m *buildManifest) {
			m.DocsOrigins = append(m.DocsOrigins, "https://unexpected.example")
		}},
		"wrong source": {change: func(_ string, m *buildManifest) { m.SourceSHA256 = strings.Repeat("1", 64) }},
		"tampered asset": {change: func(dir string, _ *buildManifest) {
			if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("tampered"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			dir, m, _ := fixtureArtifact(t, "site-input")
			m.Release, m.Commit, m.SourceSHA256 = true, commit, source
			m.AppOrigin, m.DocsOrigins = app, []string{docs, app}
			if req.change != nil {
				req.change(dir, &m)
			}
			manifest := jsonBytes(m)
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0644); err != nil {
				t.Fatal(err)
			}
			sha := digest(manifest)
			if req.sha != "" {
				sha = req.sha
				if sha == "missing" {
					sha = ""
				}
			}
			wantCommit, wantApp, wantDocs := commit, app, docs
			if req.commit != "" {
				wantCommit = req.commit
			}
			if req.app != "" {
				wantApp = req.app
			}
			if req.docs != "" {
				wantDocs = req.docs
			}
			_, _, err := verifySiteInput(dir, root, sha, wantCommit, wantApp, wantDocs)
			if (err == nil) != (name == "valid") {
				t.Fatalf("verification error = %v", err)
			}
		})
	}
}
