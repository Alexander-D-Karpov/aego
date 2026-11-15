package site

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"aego/asset"
	"aego/project"
	"aego/project/remoteexample"
)

type tryExample struct {
	Manifest, Archive, Link string
}

var exampleSlug = regexp.MustCompile(`[^a-z0-9._-]+`)

func (b *builder) writeTryExamples() error {
	if b.opt.ExamplesURL == "" {
		if b.opt.ExamplesProduction {
			return fmt.Errorf("official example publication cannot disable the examples URL")
		}
		return nil
	}
	u, err := url.Parse(b.opt.ExamplesURL)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, "/") {
		return fmt.Errorf("examples URL must be an HTTPS site root ending in /")
	}
	origin, err := remoteexample.Origin(b.opt.ExamplesURL)
	if err != nil {
		return err
	}
	version := b.opt.ExamplesVersion
	if version == "" {
		version = "1.0.0"
	}
	license := strings.TrimSpace(b.opt.ExamplesLicense)
	if license == "" {
		license = "NOASSERTION"
	}
	if b.opt.ExamplesProduction && strings.EqualFold(license, "NOASSERTION") {
		return fmt.Errorf("official example publication requires an explicit verified --examples-license")
	}
	if b.opt.ExamplesProduction && strings.TrimSpace(b.opt.ExamplesLicenseText) == "" {
		return fmt.Errorf("official example publication requires the reviewed license text")
	}
	if text := b.opt.ExamplesLicenseText; text != "" && (len(text) > 64<<10 || !utf8.ValidString(text) || strings.TrimSpace(text) == "") {
		return fmt.Errorf("example license text is empty, invalid UTF-8, or larger than 64 KiB")
	}
	b.examples = map[string]tryExample{}
	for _, e := range b.exp.Entries {
		if e.Try == "" {
			continue
		}
		if e.TrySource == nil || len(e.TrySource.Files) == 0 {
			return fmt.Errorf("%s has no complete Try source project; export the current registry", e.ID)
		}
		s := e.TrySource
		identity := e.ID
		if e.Demo != nil && e.Demo.DemoID != "" {
			identity = e.Demo.DemoID
		}
		hash := sha256.Sum256([]byte(identity))
		slug := strings.Trim(exampleSlug.ReplaceAllString(strings.ToLower(identity), "-"), ".-_")
		if len(slug) > 58 {
			slug = slug[:58]
		}
		name := fmt.Sprintf("%s-%x", slug, hash[:8])
		if slug == "" {
			name = fmt.Sprintf("example-%x", hash[:8])
		}
		rel := "examples/" + name + "/"
		manifestURL := u.ResolveReference(&url.URL{Path: rel + "manifest.json"}).String()
		archiveURL := u.ResolveReference(&url.URL{Path: rel + "project.zip"}).String()
		files := make(map[string][]byte, len(s.Files)+1)
		for name, data := range s.Files {
			files[name] = data
		}
		if b.opt.ExamplesLicenseText != "" {
			const licenseFile = "AEGO-EXAMPLE-LICENSE.txt"
			for name := range files {
				if strings.EqualFold(name, licenseFile) {
					return fmt.Errorf("%s reserves %s", identity, licenseFile)
				}
			}
			files[licenseFile] = []byte(b.opt.ExamplesLicenseText)
		}
		if _, exists := files[project.FileName]; !exists {
			meta, err := asset.ParseSourceMeta(files[s.EntryScene+".meta"], s.EntryScene+".meta")
			if err != nil {
				return fmt.Errorf("%s: entry scene metadata: %w", identity, err)
			}
			p := project.Project{Name: e.Title, Module: "game", MainScene: meta.ID,
				Window: project.WindowSettings{Width: 960, Height: 540, Title: e.Title}}
			files[project.FileName] = p.Encode()
		}
		// Archive provenance does not grant a license that the source checkout
		// did not contain. Snapshot builds state this uncertainty explicitly.
		revision := b.opt.Commit
		if revision == "" {
			revision = "not provided"
		}
		notice := fmt.Sprintf("Aego documentation example: %s\nSource revision: %s\nDocumentation build: %s\nRegistry SHA-256: %s\nExample version: %s\nLicense: %s\nOrigin: %s\n", identity, revision, b.buildID, b.registrySHA, version, license, origin)
		notice += "Documentation fingerprints identify the exported inputs, not a Git worktree or a runtime acceptance result.\n"
		if license == "NOASSERTION" {
			notice += "No license was declared in this source snapshot. This package does not add or change any permissions.\n"
		}
		if b.opt.ExamplesLicenseText != "" {
			notice += "Reviewed license text: AEGO-EXAMPLE-LICENSE.txt\n"
		}
		if _, exists := files["EXAMPLE-NOTICE.txt"]; exists {
			return fmt.Errorf("%s reserves EXAMPLE-NOTICE.txt", identity)
		}
		files["EXAMPLE-NOTICE.txt"] = []byte(notice)
		m := remoteexample.Manifest{
			Format: remoteexample.FormatVersion, ExampleID: name, Version: version,
			Engine:     remoteexample.Compatibility{Min: "0.12.1", MaxExclusive: "0.13.0"},
			EntryScene: s.EntryScene, Modules: s.Modules, AssetRefs: s.AssetRefs,
			Archive: remoteexample.Archive{URL: archiveURL}, Origin: origin, License: license,
		}
		m, _, archive, err := remoteexample.BuildPackage(m, files, manifestURL, remoteexample.Limits{})
		if err != nil {
			return fmt.Errorf("%s: %w", identity, err)
		}
		archiveRel := rel + "project-" + m.Archive.SHA256 + ".zip"
		m.Archive.URL = u.ResolveReference(&url.URL{Path: archiveRel}).String()
		manifest, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return err
		}
		manifest = append(manifest, '\n')
		if _, err := remoteexample.ParseManifest(manifest, manifestURL, remoteexample.Limits{}); err != nil {
			return err
		}
		link, err := remoteexample.MakeLink(manifestURL, "scene")
		if err != nil {
			return err
		}
		dir := filepath.Join(b.out, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(b.out, filepath.FromSlash(archiveRel)), archive, 0644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), manifest, 0644); err != nil {
			return err
		}
		b.examples[e.ID] = tryExample{Manifest: rel + "manifest.json", Archive: archiveRel, Link: link}
	}
	return nil
}
