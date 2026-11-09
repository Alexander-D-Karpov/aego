package site

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"aego/asset"
	"aego/project"
	"aego/project/remoteexample"
)

func sourceExampleFixture() *Export {
	e := fixture()
	e.Entries[0].Try = "scene version=1 {}"
	e.Entries[0].TrySource = &TrySource{
		EntryScene: "scenes/main.aescene",
		Files: map[string][]byte{
			"project.aego":             []byte("project version=1 { name=\"Example\" module=\"game\" native=false }"),
			"scenes/main.aescene":      []byte(e.Entries[0].Try),
			"scenes/main.aescene.meta": []byte("meta version=1 { id=\"10000000000000000000000000000001\" importer=\"scene\" }"),
			"scripts/demo/main.aes":    []byte("package demo\n"),
			"assets/light.png":         []byte("source pixels"),
		},
		Modules:   []string{"engine", "render2d"},
		AssetRefs: []string{"10000000000000000000000000000001"},
	}
	return e
}

func TestTryLinksDownloadCompleteSourceProjectInBothLanguages(t *testing.T) {
	e := sourceExampleFixture()
	dir := t.TempDir()
	_, err := Build(e, dir, Options{Playground: true, ExamplesURL: "https://docs.example/aego/", ExamplesVersion: "1.2.3", ExamplesLicense: "NOASSERTION", Commit: "source-snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	manifests, err := filepath.Glob(filepath.Join(dir, "examples", "*", "manifest.json"))
	if err != nil || len(manifests) != 1 {
		t.Fatal("complete source package missing", manifests, err)
	}
	manifest, err := os.ReadFile(manifests[0])
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(dir, manifests[0])
	manifestURL := "https://docs.example/aego/" + filepath.ToSlash(rel)
	m, err := remoteexample.ParseManifest(manifest, manifestURL, remoteexample.Limits{})
	if err != nil || m.Version != "1.2.3" || len(m.Modules) != 2 || len(m.AssetRefs) != 1 {
		t.Fatal("bad example manifest", m, err)
	}
	archives, _ := filepath.Glob(filepath.Join(filepath.Dir(manifests[0]), "project-*.zip"))
	if len(archives) != 1 {
		t.Fatal("content-addressed source archive missing")
	}
	data, _ := os.ReadFile(archives[0])
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != m.Archive.SHA256 {
		t.Fatal("archive differs from manifest")
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	buildBytes, err := os.ReadFile(filepath.Join(dir, "docs-build.json"))
	if err != nil {
		t.Fatal(err)
	}
	var build struct {
		BuildID        string `json:"buildID"`
		RegistrySHA256 string `json:"registrySHA256"`
	}
	if err := json.Unmarshal(buildBytes, &build); err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if want, ok := e.Entries[0].TrySource.Files[f.Name]; ok && !bytes.Equal(b, want) {
			t.Fatalf("source was changed: %s", f.Name)
		}
		if f.Name == "EXAMPLE-NOTICE.txt" && (!bytes.Contains(b, []byte("NOASSERTION")) || !bytes.Contains(b, []byte("source-snapshot"))) {
			t.Fatal("unknown license/source provenance hidden")
		}
		if f.Name == "EXAMPLE-NOTICE.txt" && (!bytes.Contains(b, []byte("Documentation build: "+build.BuildID)) || !bytes.Contains(b, []byte("Registry SHA-256: "+build.RegistrySHA256))) {
			t.Fatal("example notice is not bound to the documentation registry/build")
		}
		seen[f.Name] = true
	}
	for name := range e.Entries[0].TrySource.Files {
		if !seen[name] {
			t.Fatalf("source dependency missing from package: %s", name)
		}
	}
	linkRE := regexp.MustCompile(`class="try-open button" href="([^"]+)"`)
	for _, page := range []string{"welcome.html", "ru/welcome.html"} {
		b, err := os.ReadFile(filepath.Join(dir, page))
		if err != nil {
			t.Fatal(err)
		}
		match := linkRE.FindSubmatch(b)
		if match == nil {
			t.Fatalf("%s has no genuine Open in Aego link", page)
		}
		link, err := remoteexample.ParseLink(html.UnescapeString(string(match[1])))
		if err != nil || link.ManifestURL != manifestURL || link.Mode != "scene" {
			t.Fatal("page deep link does not match the complete source manifest", link, err)
		}
		for _, marker := range []string{`class="try-project button"`, ` download`, `class="try-download"`, `data-example-archive=`} {
			if !bytes.Contains(b, []byte(marker)) {
				t.Fatalf("page omits full project fallback or edited scene export: %s", marker)
			}
		}
	}
}

func TestTrySourcePackagingFailsOnMissingDependenciesAndUnknownProductionLicense(t *testing.T) {
	opt := Options{ExamplesURL: "https://docs.example/"}
	for _, test := range []struct {
		name   string
		change func(*Export, *Options)
	}{
		{"missing source", func(e *Export, _ *Options) { e.Entries[0].TrySource = nil }},
		{"missing entry", func(e *Export, _ *Options) { delete(e.Entries[0].TrySource.Files, "scenes/main.aescene") }},
		{"unsafe file", func(e *Export, _ *Options) { e.Entries[0].TrySource.Files["../outside.txt"] = []byte("bad") }},
		{"unknown production license", func(_ *Export, o *Options) { o.ExamplesProduction = true }},
		{"disabled production examples", func(_ *Export, o *Options) {
			o.ExamplesURL, o.ExamplesLicense, o.ExamplesProduction = "", "LicenseRef-Reviewed-Test-Example", true
		}},
		{"insecure origin", func(_ *Export, o *Options) { o.ExamplesURL = "http://docs.example/" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e, o := sourceExampleFixture(), opt
			test.change(e, &o)
			if _, err := Build(e, t.TempDir(), o); err == nil {
				t.Fatal("invalid official source package accepted")
			}
		})
	}
}

func TestTrySourceHostCreatesProjectUsingCanonicalEncoder(t *testing.T) {
	e := sourceExampleFixture()
	s := e.Entries[0].TrySource
	delete(s.Files, project.FileName)
	dir := t.TempDir()
	if _, err := Build(e, dir, Options{ExamplesURL: "https://docs.example/"}); err != nil {
		t.Fatal(err)
	}
	archives, err := filepath.Glob(filepath.Join(dir, "examples", "*", "project-*.zip"))
	if err != nil || len(archives) != 1 {
		t.Fatal("source package missing", archives, err)
	}
	r, err := zip.OpenReader(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	root := t.TempDir()
	found := false
	for _, f := range r.File {
		if f.Name != project.FileName {
			continue
		}
		stream, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, project.FileName), data, 0644); err != nil {
			t.Fatal(err)
		}
		found = true
	}
	if !found {
		t.Fatal("packaging host omitted project.aego")
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := asset.ParseSourceMeta(s.Files[s.EntryScene+".meta"], s.EntryScene+".meta")
	if err != nil {
		t.Fatal(err)
	}
	if p.MainScene != meta.ID || p.Module != "game" || p.Native || p.Window.Width != 960 || p.Window.Height != 540 || p.Name != e.Entries[0].Title {
		t.Fatalf("host changed the existing entry identity or produced an incompatible project: %+v", p)
	}
	if _, exists := s.Files[project.FileName]; exists {
		t.Fatal("packaging mutated the registry source snapshot")
	}
}

func TestRealExportHasCompleteSourcePackageForEveryPublicComponent(t *testing.T) {
	file := os.Getenv("AEGO_DOCS_EXPORT")
	if file == "" {
		t.Skip("run make docs-check to supply the current real registry export")
	}
	e, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range e.Entries {
		if entry.Component == nil {
			continue
		}
		count++
		if entry.TrySource == nil || entry.TrySource.EntryScene == "" || len(entry.TrySource.Files) == 0 {
			t.Errorf("%s has no complete editable source project", entry.ID)
		}
	}
	if count == 0 {
		t.Fatal("no public component registry entries tested")
	}
	dir := t.TempDir()
	if _, err := Build(e, dir, Options{ExamplesURL: "https://docs.example/", ExamplesVersion: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "examples", "*", "manifest.json"))
	if err != nil || len(files) < count {
		t.Fatalf("source packages=%d public components=%d err=%v", len(files), count, err)
	}
	t.Logf("%d public components have validated complete source packages (%d packages including guides)", count, len(files))
	for _, file := range files {
		b, _ := os.ReadFile(file)
		if strings.Contains(string(b), ".git/") || strings.Contains(string(b), "native/") {
			t.Fatalf("package included excluded repository data: %s", file)
		}
	}
}
