package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep this list independent of sourceHash so additions to its production source
// inventory require an explicit fixture update rather than being silently lost.
var siteInputSourceDirs = []string{
	"art", "tools/artstudio", "anim", "asset", "contracts", "core", "engine", "format", "gpu",
	"modules/render2d", "modules/particles", "platform", "render", "save", "scene", "schema", "script", "vfs",
}

func siteInputSourceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range siteInputSourceDirs {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "input.go"), []byte("source for "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("source for "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestProductionSiteInputRequiresPinnedRelease(t *testing.T) {
	const app = "https://aeditor.akarpov.ru"
	const docs = "https://aego.akarpov.ru"
	commit := strings.Repeat("a", 40)
	root := siteInputSourceFixture(t)
	source, err := sourceHash(root)
	if err != nil {
		t.Fatal(err)
	}

	type request struct {
		sha, commit, app, docs string
		change                 func(string, *buildManifest)
	}
	tests := map[string]request{
		"valid":               {},
		"missing hash":        {sha: "missing"},
		"wrong hash":          {sha: strings.Repeat("0", 64)},
		"short commit":        {commit: "abcd"},
		"wrong commit":        {commit: strings.Repeat("b", 40)},
		"wrong app origin":    {app: "https://unexpected.example"},
		"wrong studio origin": {app: "https://aestudio.akarpov.ru"},
		"wrong docs origin":   {docs: "https://unexpected.example"},
		"snapshot":            {change: func(_ string, m *buildManifest) { m.Release = false }},
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

func TestSiteInputSourceFixtureIsHashed(t *testing.T) {
	inputs := []string{"go.mod", "go.sum"}
	for _, dir := range siteInputSourceDirs {
		inputs = append(inputs, filepath.Join(dir, "input.go"))
	}
	for _, name := range inputs {
		t.Run(name, func(t *testing.T) {
			root := siteInputSourceFixture(t)
			before, err := sourceHash(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, name), []byte("modified "+name), 0644); err != nil {
				t.Fatal(err)
			}
			after, err := sourceHash(root)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Fatalf("sourceHash ignored %s", name)
			}
		})
	}
}

func TestProductionSiteInputRejectsChangedSource(t *testing.T) {
	const app = "https://aeditor.akarpov.ru"
	const docs = "https://aego.akarpov.ru"
	commit := strings.Repeat("a", 40)
	root := siteInputSourceFixture(t)
	source, err := sourceHash(root)
	if err != nil {
		t.Fatal(err)
	}
	dir, m, _ := fixtureArtifact(t, "changed-site-source")
	m.Release, m.Commit, m.SourceSHA256 = true, commit, source
	m.AppOrigin, m.DocsOrigins = app, []string{docs, app}
	manifest := jsonBytes(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range siteInputSourceDirs {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name, "input.go")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.WriteFile(path, original, 0644); err != nil {
					t.Error(err)
				}
			})
			if err := os.WriteFile(path, []byte("changed"), 0644); err != nil {
				t.Fatal(err)
			}
			_, _, err = verifySiteInput(dir, root, digest(manifest), commit, app, docs)
			if err == nil || !strings.Contains(err.Error(), "site source subset differs from the artifact") {
				t.Fatalf("changed %s accepted or wrong failure: %v", name, err)
			}
		})
	}
}
