package site

import (
	"crypto/sha256"
	hexenc "encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

func digest(data []byte) string {
	h := sha256.Sum256(data)
	return hexenc.EncodeToString(h[:])
}

func (b *builder) docsVersion() string {
	v := b.opt.Version
	if v == "" {
		v = "unversioned"
	}
	channel := b.opt.Channel
	if channel == "" {
		channel = "development"
	}
	return "Aego " + v + " · " + channel
}

func (b *builder) markdownBuildNote() string {
	return "> " + b.docsVersion() + "; docs build `" + b.buildID + "`. [Build manifest](/docs-build.json)."
}

// identifyBuild fingerprints the registry snapshot and presentation inputs.
// This is a documentation build identity, not a Git commit or worktree hash.
func (b *builder) identifyBuild() error {
	registry, err := json.Marshal(b.exp)
	if err != nil {
		return err
	}
	b.registrySHA = digest(registry)
	inputs, err := json.Marshal(struct {
		Registry, Assets, Layout string
		Options                  Options
	}{b.registrySHA, b.assetV, digest([]byte(layoutSrc)), b.opt})
	if err != nil {
		return err
	}
	b.buildID = digest(inputs)
	return nil
}

func (b *builder) writeBuildManifest() error {
	pages := make(map[string]string, len(b.order))
	for _, id := range b.order {
		pages[id] = cleanURL(b.urls[id])
	}
	data, err := json.MarshalIndent(struct {
		Schema         int               `json:"schema"`
		BuildID        string            `json:"buildID"`
		RegistrySHA256 string            `json:"registrySHA256"`
		Version        string            `json:"version"`
		Commit         string            `json:"commit"`
		IdentityScope  string            `json:"identityScope"`
		Pages          map[string]string `json:"pages"`
	}{1, b.buildID, b.registrySHA, b.docsVersion(), b.opt.Commit,
		"Registry snapshot and presentation inputs; not a Git worktree fingerprint or runtime acceptance result.", pages}, "", "  ")
	if err != nil {
		return err
	}
	// This is the same registry snapshot the HTML/Markdown/search renderer used.
	registry, err := json.Marshal(b.exp)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(b.out, "docs-registry.json"), registry, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.out, "docs-build.json"), append(data, '\n'), 0o644)
}
