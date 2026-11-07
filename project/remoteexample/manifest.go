// Package remoteexample installs data-only, versioned example projects.
// No operation executes code, installs modules, launches the engine, or mutates
// an existing local copy. A caller must request those actions separately.
package remoteexample

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"aego/project"
)

const FormatVersion = 1

type Limits struct {
	ManifestBytes  int64
	ArchiveBytes   int64
	ExpandedBytes  int64
	FileBytes      int64
	Files          int
	ExpansionRatio uint64
}

func DefaultLimits() Limits { return Limits{1 << 20, 64 << 20, 256 << 20, 32 << 20, 8192, 500} }
func normalizeLimits(l Limits) Limits {
	d := DefaultLimits()
	if l.ManifestBytes > 0 {
		d.ManifestBytes = min(d.ManifestBytes, l.ManifestBytes)
	}
	if l.ArchiveBytes > 0 {
		d.ArchiveBytes = min(d.ArchiveBytes, l.ArchiveBytes)
	}
	if l.ExpandedBytes > 0 {
		d.ExpandedBytes = min(d.ExpandedBytes, l.ExpandedBytes)
	}
	if l.FileBytes > 0 {
		d.FileBytes = min(d.FileBytes, l.FileBytes)
	}
	if l.Files > 0 {
		d.Files = min(d.Files, l.Files)
	}
	if l.ExpansionRatio > 0 {
		d.ExpansionRatio = min(d.ExpansionRatio, l.ExpansionRatio)
	}
	return d
}

type File struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Archive struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Compatibility struct {
	Min          string `json:"min"`
	MaxExclusive string `json:"maxExclusive"`
}
type Manifest struct {
	Format     int           `json:"format"`
	ExampleID  string        `json:"exampleID"`
	Version    string        `json:"version"`
	Engine     Compatibility `json:"engine"`
	EntryScene string        `json:"entryScene"`
	Modules    []string      `json:"requiredModules"`
	Files      []File        `json:"files"`
	Archive    Archive       `json:"archive"`
	License    string        `json:"license"`
	Origin     string        `json:"origin"`
}

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)
var versionRE = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func version(v string) ([3]uint64, error) {
	var out [3]uint64
	if !versionRE.MatchString(v) {
		return out, fmt.Errorf("invalid release version %q", v)
	}
	for i, p := range strings.Split(v, ".") {
		n, e := strconv.ParseUint(p, 10, 32)
		if e != nil {
			return out, e
		}
		out[i] = n
	}
	return out, nil
}
func less(a, b [3]uint64) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
func (m Manifest) CheckEngine(current string, installed []string) error {
	v, e := version(current)
	if e != nil {
		return e
	}
	lo, e := version(m.Engine.Min)
	if e != nil {
		return e
	}
	hi, e := version(m.Engine.MaxExclusive)
	if e != nil {
		return e
	}
	if less(v, lo) || !less(v, hi) {
		return fmt.Errorf("example needs engine [%s,%s), got %s", m.Engine.Min, m.Engine.MaxExclusive, current)
	}
	have := map[string]bool{}
	for _, id := range installed {
		have[id] = true
	}
	for _, id := range m.Modules {
		if !have[id] {
			return fmt.Errorf("required installed module is unavailable: %s", id)
		}
	}
	return nil
}

func httpsURL(raw string) (*url.URL, error) {
	if len(raw) > 8192 {
		return nil, fmt.Errorf("URL too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, fmt.Errorf("an absolute HTTPS URL without credentials or fragment is required")
	}
	if strings.ContainsAny(u.Host, "\\% \t\r\n") || strings.HasSuffix(u.Hostname(), ".") {
		return nil, fmt.Errorf("ambiguous HTTPS host")
	}
	host := u.Hostname()
	if _, err := netip.ParseAddr(host); err != nil {
		// Accept DNS names in their explicit ASCII/IDNA form, never patterns,
		// escaped hosts, underscores or ambiguous empty labels.
		if len(host) > 253 || strings.HasSuffix(u.Host, ":") {
			return nil, fmt.Errorf("invalid HTTPS hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, fmt.Errorf("invalid HTTPS hostname")
			}
			for _, c := range label {
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
					return nil, fmt.Errorf("invalid HTTPS hostname")
				}
			}
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("empty HTTPS port")
	}
	if p := u.Port(); p != "" && p != "443" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("invalid HTTPS port")
		}
	}
	return u, nil
}
func originOf(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if u.Port() != "" && u.Port() != "443" {
		host += ":" + u.Port()
	}
	return "https://" + host
}
func Origin(raw string) (string, error) {
	u, e := httpsURL(raw)
	if e != nil {
		return "", e
	}
	return originOf(u), nil
}
func digestOK(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == strings.ToLower(s)
}

// The transport profile deliberately permits only portable ASCII relative
// paths. This prevents Windows device names, alternate data streams, case-only
// collisions and Unicode-normalization aliases across supported desktops.
func cleanPath(name string) error {
	if len(name) == 0 || len(name) > 240 || name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00<>\"|?*") {
		return fmt.Errorf("unsafe package path %q", name)
	}
	for _, c := range name {
		if c < 32 || c > 126 {
			return fmt.Errorf("non-portable package path %q", name)
		}
	}
	for _, part := range strings.Split(name, "/") {
		low := strings.ToLower(part)
		if part == "." || part == ".." || part == "" || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.HasPrefix(part, " ") {
			return fmt.Errorf("unsafe path component %q", part)
		}
		base := strings.SplitN(low, ".", 2)[0]
		if base == "con" || base == "prn" || base == "aux" || base == "nul" || base == "clock$" || (len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '0' && base[3] <= '9') {
			return fmt.Errorf("reserved path component %q", part)
		}
		if low == ".git" || low == ".hg" || low == ".svn" || low == ".aego" || low == "node_modules" || low == ".env" || strings.HasPrefix(low, ".env.") {
			return fmt.Errorf("forbidden package directory/file %q", part)
		}
	}
	return nil
}
func allowedFile(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".aego", ".aetext", ".aeproj", ".aeproject", ".aescene", ".aeprefab", ".aes", ".aeanim", ".aeanimator", ".aemat", ".aeshader", ".aeshaderinc", ".aepreset", ".aetry", ".aeart", ".aemixer", ".png", ".jpg", ".jpeg", ".webp", ".ogg", ".wav", ".mp3", ".flac", ".ttf", ".otf", ".json", ".meta", ".txt", ".md", ".atlas", ".ase", ".aseprite", ".glsl", ".frag", ".vert", ".lut", ".cube", ".csv":
		return true
	}
	switch strings.ToLower(path.Base(name)) {
	case "license", "copying", "notice":
		return true
	}
	return false
}
func (m Manifest) Validate(source string, lim Limits) error {
	l := normalizeLimits(lim)
	if m.Format != FormatVersion || !slugRE.MatchString(m.ExampleID) {
		return fmt.Errorf("unsupported manifest or invalid example ID")
	}
	if _, err := version(m.Version); err != nil {
		return err
	}
	lo, e := version(m.Engine.Min)
	if e != nil {
		return e
	}
	hi, e := version(m.Engine.MaxExclusive)
	if e != nil {
		return e
	}
	if !less(lo, hi) {
		return fmt.Errorf("invalid engine interval")
	}
	so, e := Origin(source)
	if e != nil {
		return e
	}
	mo, e := Origin(m.Origin)
	if e != nil || m.Origin != mo || mo != so {
		return fmt.Errorf("manifest provenance does not match its HTTPS origin")
	}
	au, e := httpsURL(m.Archive.URL)
	if e != nil || originOf(au) != so {
		return fmt.Errorf("archive must stay on the approved HTTPS origin")
	}
	if m.Archive.Size < 1 || m.Archive.Size > l.ArchiveBytes || !digestOK(m.Archive.SHA256) {
		return fmt.Errorf("invalid archive size or SHA-256")
	}
	if strings.TrimSpace(m.License) == "" || len(m.License) > 2048 {
		return fmt.Errorf("license is required")
	}
	if len(m.Files) == 0 || len(m.Files) > l.Files {
		return fmt.Errorf("file count exceeds limits")
	}
	mods := map[string]bool{}
	for _, id := range m.Modules {
		if !slugRE.MatchString(id) || mods[id] {
			return fmt.Errorf("invalid or duplicate module ID")
		}
		mods[id] = true
	}
	if len(m.Modules) > 256 {
		return fmt.Errorf("too many required modules")
	}
	names := map[string]bool{}
	spelling := map[string]string{}
	var total int64
	for _, f := range m.Files {
		if e := cleanPath(f.Path); e != nil {
			return e
		}
		if !allowedFile(f.Path) || f.Path == "remote-example.receipt.json" {
			return fmt.Errorf("not an allowed project resource: %s", f.Path)
		}
		for p := f.Path; p != "."; p = path.Dir(p) {
			key := strings.ToLower(p)
			if prior, exists := spelling[key]; exists && prior != p {
				return fmt.Errorf("case-colliding resource path: %s", p)
			}
			spelling[key] = p
		}
		low := strings.ToLower(f.Path)
		if names[low] {
			return fmt.Errorf("duplicate or case-colliding file: %s", f.Path)
		}
		names[low] = true
		if f.Size < 0 || f.Size > l.FileBytes || f.Size > l.ExpandedBytes-total || !digestOK(f.SHA256) {
			return fmt.Errorf("invalid file size or digest: %s", f.Path)
		}
		total += f.Size
	}
	for _, f := range m.Files {
		for parent := path.Dir(f.Path); parent != "."; parent = path.Dir(parent) {
			if names[strings.ToLower(parent)] {
				return fmt.Errorf("file/directory collision: %s", parent)
			}
		}
	}
	entry, projectFile := false, false
	for _, f := range m.Files {
		if f.Path == m.EntryScene {
			entry = true
		}
		if f.Path == project.FileName {
			projectFile = true
		}
	}
	if !entry || !strings.HasSuffix(m.EntryScene, ".aescene") {
		return fmt.Errorf("entry scene is not a listed .aescene")
	}
	if !projectFile {
		return fmt.Errorf("project.aego is not a listed project resource")
	}
	return nil
}
func ParseManifest(data []byte, source string, lim Limits) (Manifest, error) {
	var m Manifest
	l := normalizeLimits(lim)
	if int64(len(data)) > l.ManifestBytes {
		return m, fmt.Errorf("manifest exceeds limits")
	}
	// Reject duplicate keys too; encoding/json alone accepts last-wins fields.
	if err := uniqueJSON(data); err != nil {
		return m, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return m, fmt.Errorf("trailing manifest data")
	}
	return m, m.Validate(source, l)
}
func uniqueJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return fmt.Errorf("JSON nesting exceeds limits")
		}
		t, e := d.Token()
		if e != nil {
			return e
		}
		v, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		switch v {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || seen[strings.ToLower(s)] {
					return fmt.Errorf("duplicate JSON key")
				}
				seen[strings.ToLower(s)] = true
				if e := walk(depth + 1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e := walk(depth + 1); e != nil {
					return e
				}
			}
		default:
			return fmt.Errorf("invalid JSON delimiter")
		}
		_, e = d.Token()
		return e
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
