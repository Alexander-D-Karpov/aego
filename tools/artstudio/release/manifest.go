package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const manifestVersion = 1
const embedProtocolVersion = 1
const maxArtifactBytes int64 = 96 << 20
const maxFileBytes int64 = 64 << 20

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hashedFile = regexp.MustCompile(`\.[0-9a-f]{16}\.`)

type resource struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	MIME   string `json:"mime"`
}
type example struct {
	ID          string            `json:"id"`
	Version     int               `json:"version"`
	Title       map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	File        string            `json:"file"`
	Thumbnail   string            `json:"thumbnail"`
	SHA256      string            `json:"sha256"`
	Bytes       int               `json:"bytes"`
}
type buildManifest struct {
	Schema          int        `json:"schema"`
	AegoVersion     string     `json:"aegoVersion"`
	StudioVersion   string     `json:"studioVersion"`
	DocumentVersion int        `json:"documentVersion"`
	ProtocolVersion int        `json:"protocolVersion"`
	BuildID         string     `json:"buildID"`
	Commit          string     `json:"commit,omitempty"`
	SourceSHA256    string     `json:"sourceSHA256"`
	BuiltAt         string     `json:"builtAt"`
	Toolchain       string     `json:"toolchain"`
	Release         bool       `json:"release"`
	AppOrigin       string     `json:"appOrigin"`
	DocsOrigins     []string   `json:"docsOrigins"`
	WASM            resource   `json:"wasm"`
	Examples        []example  `json:"examples"`
	Files           []resource `json:"files"`
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func mimeFor(p string) string {
	switch path.Ext(p) {
	case ".wasm":
		return "application/wasm"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".png":
		return "image/png"
	case ".svg":
		return "image/svg+xml"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
func validPath(p string) bool {
	return p != "" && !strings.ContainsAny(p, "\\\x00:\n\r?#%") && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != "." && p != ".." && !strings.HasPrefix(p, "../")
}
func validOrigin(raw string, release bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.String() != raw {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	return !release && u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}
func decodeManifest(b []byte) (buildManifest, error) {
	var m buildManifest
	if len(b) > 2<<20 {
		return m, errors.New("manifest size limit")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return m, errors.New("trailing manifest data")
	}
	if m.Schema != manifestVersion || m.ProtocolVersion != embedProtocolVersion || m.DocumentVersion != 1 || !identifier.MatchString(m.BuildID) || !shaPattern.MatchString(m.SourceSHA256) || m.BuiltAt == "" || m.Toolchain == "" {
		return m, errors.New("unsupported or incomplete manifest")
	}
	if m.Release && !commitPattern.MatchString(m.Commit) {
		return m, errors.New("release requires exact Git commit")
	}
	if !validOrigin(m.AppOrigin, m.Release) || len(m.DocsOrigins) == 0 || len(m.DocsOrigins) > 8 {
		return m, errors.New("invalid origins")
	}
	for _, o := range m.DocsOrigins {
		if !validOrigin(o, m.Release) {
			return m, errors.New("invalid documentation origin")
		}
	}
	if len(m.Files) < 6 || len(m.Files) > 4096 {
		return m, errors.New("invalid file count")
	}
	seen := map[string]resource{}
	var total int64
	for _, f := range m.Files {
		if !validPath(f.Path) || f.Path == "manifest.json" || f.Bytes < 0 || f.Bytes > maxFileBytes || !shaPattern.MatchString(f.SHA256) || f.MIME != mimeFor(f.Path) {
			return m, fmt.Errorf("invalid file record %q", f.Path)
		}
		if _, ok := seen[f.Path]; ok {
			return m, fmt.Errorf("duplicate file %s", f.Path)
		}
		seen[f.Path] = f
		total += f.Bytes
	}
	if total > maxArtifactBytes {
		return m, errors.New("artifact size limit")
	}
	for _, p := range []string{"index.html", "editor/index.html", "embed/index.html"} {
		if _, ok := seen[p]; !ok {
			return m, fmt.Errorf("missing route %s", p)
		}
	}
	if f, ok := seen[m.WASM.Path]; !ok || f != m.WASM || !strings.HasSuffix(f.Path, ".wasm") {
		return m, errors.New("WASM manifest mismatch")
	}
	ex := map[string]bool{}
	for _, e := range m.Examples {
		f, ok := seen[e.File]
		if !identifier.MatchString(e.ID) || ex[e.ID] || e.Version < 1 || !ok || f.SHA256 != e.SHA256 || f.Bytes != int64(e.Bytes) || e.Title["en"] == "" || e.Title["ru"] == "" {
			return m, errors.New("invalid example")
		}
		ex[e.ID] = true
		if _, ok := seen[e.Thumbnail]; !ok {
			return m, errors.New("missing thumbnail")
		}
	}
	return m, nil
}
func noSymlinkPath(p string) error {
	p, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	for {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path refused: %s", p)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return nil
		}
		p = parent
	}
}
func readBounded(file string, max int64) ([]byte, error) {
	st, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > max {
		return nil, fmt.Errorf("non-regular or oversized file: %s", file)
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("file grew beyond limit")
	}
	return b, nil
}
func verify(dir, expected string) (buildManifest, string, error) {
	var empty buildManifest
	if err := noSymlinkPath(dir); err != nil {
		return empty, "", err
	}
	b, err := readBounded(filepath.Join(dir, "manifest.json"), 2<<20)
	if err != nil {
		return empty, "", err
	}
	hash := digest(b)
	if expected != "" && hash != expected {
		return empty, hash, errors.New("manifest SHA-256 mismatch")
	}
	m, err := decodeManifest(b)
	if err != nil {
		return m, hash, err
	}
	files := map[string]resource{}
	for _, f := range m.Files {
		files[f.Path] = f
	}
	count := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink refused: %s", p)
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(dir, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		f, ok := files[rel]
		if !ok {
			return fmt.Errorf("unlisted artifact file: %s", rel)
		}
		b, e := readBounded(p, maxFileBytes)
		if e != nil {
			return e
		}
		if int64(len(b)) != f.Bytes || digest(b) != f.SHA256 {
			return fmt.Errorf("file integrity mismatch: %s", rel)
		}
		count++
		return nil
	})
	if err == nil && count != len(files) {
		err = errors.New("artifact is incomplete")
	}
	return m, hash, err
}
func inventory(dir string) ([]resource, error) {
	var files []resource
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink in output")
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "manifest.json" {
			return nil
		}
		b, e := readBounded(p, maxFileBytes)
		if e != nil {
			return e
		}
		files = append(files, resource{rel, int64(len(b)), digest(b), mimeFor(rel)})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}
func writeNew(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	return ce
}
func jsonBytes(v any) []byte {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		panic(e)
	}
	return append(b, '\n')
}
