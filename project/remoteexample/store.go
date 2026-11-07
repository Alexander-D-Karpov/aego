package remoteexample

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var copyIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}-[0-9a-f]{24}$`)

const receiptName = "remote-example.receipt.json"

type Store struct {
	root   string
	limits Limits
}
type Copy struct {
	ID          string    `json:"id"`
	ManifestURL string    `json:"manifestURL"`
	Manifest    Manifest  `json:"manifest"`
	Created     time.Time `json:"created"`
	Directory   string    `json:"-"`
	Entry       string    `json:"-"`
}

func NewStore(root string, limits Limits) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("local store root is required")
	}
	abs, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = ensureDirectory(abs); e != nil {
		return nil, e
	}
	for _, part := range []string{"cache", "projects"} {
		if e = ensureDirectory(filepath.Join(abs, part)); e != nil {
			return nil, e
		}
	}
	return &Store{root: abs, limits: normalizeLimits(limits)}, nil
}
func noLinks(name string) error {
	abs, e := filepath.Abs(name)
	if e != nil {
		return e
	}
	for current := abs; ; current = filepath.Dir(current) {
		st, e := os.Lstat(current)
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path rejected: %s", current)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}
func ensureDirectory(name string) error {
	// Check existing ancestors before MkdirAll and recheck the result. New
	// extraction trees are private; no archive entry can create symlinks.
	for current := name; ; current = filepath.Dir(current) {
		_, e := os.Lstat(current)
		if e == nil {
			if e = noLinks(current); e != nil {
				return e
			}
			break
		}
		if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if filepath.Dir(current) == current {
			return e
		}
	}
	if e := os.MkdirAll(name, 0700); e != nil {
		return e
	}
	if e := noLinks(name); e != nil {
		return e
	}
	st, e := os.Stat(name)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("example store must be a non-writable-by-others directory: %s", name)
	}
	return nil
}
func nonce() (string, error) {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Store) lock() (func(), error) {
	if e := noLinks(s.root); e != nil {
		return nil, e
	}
	p := filepath.Join(s.root, ".install-lock")
	if e := os.Mkdir(p, 0700); e != nil {
		return nil, fmt.Errorf("example store is busy or has an interrupted install lock: %w", e)
	}
	return func() { _ = os.Remove(p) }, nil
}
func hashFile(ctx context.Context, p string, size int64) (string, error) {
	if e := noLinks(p); e != nil {
		return "", e
	}
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return "", e
	}
	if !st.Mode().IsRegular() || st.Size() != size {
		return "", fmt.Errorf("cached size mismatch")
	}
	h := sha256.New()
	n, e := io.Copy(h, &contextReader{ctx, io.LimitReader(f, size+1)})
	if e != nil {
		return "", e
	}
	if n != size {
		return "", fmt.Errorf("file changed while reading")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}

func (s *Store) archive(ctx context.Context, c *Client, m Manifest) (string, error) {
	cached := filepath.Join(s.root, "cache", m.Archive.SHA256+".zip")
	if digest, e := hashFile(ctx, cached, m.Archive.Size); e == nil && digest == m.Archive.SHA256 {
		return cached, nil
	}
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if e := noLinks(filepath.Dir(cached)); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(filepath.Dir(cached), ".download-*")
	if e != nil {
		return "", e
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	h := sha256.New()
	n, e := c.get(ctx, m.Archive.URL, m.Archive.Size, io.MultiWriter(f, h))
	if e != nil {
		return "", e
	}
	if n != m.Archive.Size || hex.EncodeToString(h.Sum(nil)) != m.Archive.SHA256 {
		return "", fmt.Errorf("archive size or SHA-256 mismatch")
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	// Only a content-addressed cache entry can be replaced, never a project.
	if st, e := os.Lstat(cached); e == nil && !st.Mode().IsRegular() {
		return "", fmt.Errorf("unsafe existing cache entry")
	}
	if e = os.Rename(temp, cached); e != nil {
		return "", e
	}
	return cached, nil
}

func (s *Store) Clone(ctx context.Context, c *Client, source, approved string) (Copy, error) {
	var out Copy
	if c == nil {
		return out, fmt.Errorf("remote client required")
	}
	m, e := c.FetchManifest(ctx, source, approved)
	if e != nil {
		return out, e
	}
	if e = m.Validate(source, s.limits); e != nil {
		return out, e
	}
	unlock, e := s.lock()
	if e != nil {
		return out, e
	}
	defer unlock()
	packed, e := s.archive(ctx, c, m)
	if e != nil {
		return out, e
	}
	projects := filepath.Join(s.root, "projects")
	if e = noLinks(projects); e != nil {
		return out, e
	}
	stage, e := os.MkdirTemp(projects, ".extract-*")
	if e != nil {
		return out, e
	}
	defer os.RemoveAll(stage)
	if e = extract(ctx, packed, stage, m, s.limits); e != nil {
		return out, e
	}
	id, e := nonce()
	if e != nil {
		return out, e
	}
	id = m.ExampleID + "-" + id
	out = Copy{ID: id, ManifestURL: source, Manifest: m, Created: time.Now().UTC()}
	receipt, e := json.Marshal(out)
	if e != nil {
		return Copy{}, e
	}
	if e = writeNew(filepath.Join(stage, receiptName), receipt); e != nil {
		return Copy{}, e
	}
	target := filepath.Join(projects, id)
	if _, e = os.Lstat(target); !errors.Is(e, os.ErrNotExist) {
		return Copy{}, fmt.Errorf("local copy path already exists")
	}
	if e = ctx.Err(); e != nil {
		return Copy{}, e
	}
	if e = os.Rename(stage, target); e != nil {
		return Copy{}, e
	}
	out.Directory = target
	out.Entry = filepath.Join(target, filepath.FromSlash(m.EntryScene))
	return out, nil
}
func writeNew(name string, data []byte) error {
	f, e := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	return f.Close()
}

func extract(ctx context.Context, archive, dst string, m Manifest, l Limits) error {
	if e := preflightZip(archive, l.Files*2+32, l.ArchiveBytes); e != nil {
		return e
	}
	zr, e := zip.OpenReader(archive)
	if e != nil {
		return e
	}
	defer zr.Close()
	if len(zr.File) > l.Files*2+32 {
		return fmt.Errorf("archive has too many entries")
	}
	files := map[string]File{}
	parents := map[string]bool{}
	for _, f := range m.Files {
		files[f.Path] = f
		for p := path.Dir(f.Path); p != "."; p = path.Dir(p) {
			parents[p] = true
		}
	}
	seen := map[string]bool{}
	verified := map[string]bool{}
	for _, entry := range zr.File {
		if e = ctx.Err(); e != nil {
			return e
		}
		name := entry.Name
		isDir := strings.HasSuffix(name, "/")
		if isDir {
			name = strings.TrimSuffix(name, "/")
		}
		if e = cleanPath(name); e != nil {
			return e
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			return fmt.Errorf("duplicate archive entry: %s", name)
		}
		seen[lower] = true
		mode := entry.Mode()
		if mode&os.ModeSymlink != 0 || mode&(os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice) != 0 || entry.Flags&1 != 0 {
			return fmt.Errorf("unsafe archive entry type: %s", name)
		}
		if isDir {
			if !mode.IsDir() || !parents[name] || entry.UncompressedSize64 != 0 {
				return fmt.Errorf("unexpected archive directory: %s", name)
			}
			continue
		}
		if !mode.IsRegular() {
			return fmt.Errorf("not a regular resource: %s", name)
		}
		f, ok := files[name]
		if !ok {
			return fmt.Errorf("unlisted archive file: %s", name)
		}
		if entry.UncompressedSize64 != uint64(f.Size) {
			return fmt.Errorf("file header size mismatch: %s", name)
		}
		if f.Size > 0 && (entry.CompressedSize64 == 0 || entry.UncompressedSize64/entry.CompressedSize64 > l.ExpansionRatio) {
			return fmt.Errorf("excessive compression ratio: %s", name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		output, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		input, e := entry.Open()
		if e != nil {
			output.Close()
			return e
		}
		hash := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(output, hash), &contextReader{ctx, io.LimitReader(input, f.Size+1)})
		inputErr := input.Close()
		syncErr := output.Sync()
		closeErr := output.Close()
		for _, err := range []error{copyErr, inputErr, syncErr, closeErr} {
			if err != nil {
				return err
			}
		}
		if n != f.Size || hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
			return fmt.Errorf("file size or SHA-256 mismatch: %s", name)
		}
		verified[name] = true
	}
	if len(verified) != len(files) {
		return fmt.Errorf("archive is missing declared files")
	}
	return nil
}

func (s *Store) Copies() ([]Copy, error) {
	root := filepath.Join(s.root, "projects")
	if e := noLinks(root); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(root)
	if e != nil {
		return nil, e
	}
	var out []Copy
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		c, e := s.Reopen(entry.Name())
		if e != nil {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
func (s *Store) Reopen(id string) (Copy, error) {
	var c Copy
	if !copyIDRE.MatchString(id) {
		return c, fmt.Errorf("invalid local copy ID")
	}
	if e := cleanPath(id); e != nil || strings.Contains(id, "/") {
		return c, fmt.Errorf("invalid local copy ID")
	}
	dir := filepath.Join(s.root, "projects", id)
	name := filepath.Join(dir, receiptName)
	if e := noLinks(name); e != nil {
		return c, e
	}
	f, e := os.Open(name)
	if e != nil {
		return c, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, s.limits.ManifestBytes*2+1))
	if e != nil {
		return c, e
	}
	if int64(len(data)) > s.limits.ManifestBytes*2 {
		return c, fmt.Errorf("local receipt exceeds limits")
	}
	if e = uniqueJSON(data); e != nil {
		return c, e
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&c); e != nil {
		return c, e
	}
	if c.ID != id {
		return c, fmt.Errorf("copy identity mismatch")
	}
	if e = c.Manifest.Validate(c.ManifestURL, s.limits); e != nil {
		return c, e
	}
	c.Directory = dir
	c.Entry = filepath.Join(dir, filepath.FromSlash(c.Manifest.EntryScene))
	// A local copy is editable, so do not re-hash the user's changes. It must
	// still be safe for the editor's asset scan and project/cache writes. A
	// previously edited tree can otherwise contain links introduced after the
	// archive was extracted (including project.aego and asset directories).
	if e = noLinks(filepath.Join(dir, "project.aego")); e != nil {
		return c, e
	}
	if e = filepath.WalkDir(dir, func(name string, item os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path rejected: %s", name)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular project resource rejected: %s", name)
		}
		return nil
	}); e != nil {
		return c, e
	}
	if e = noLinks(c.Entry); e != nil {
		return c, e
	}
	st, e := os.Stat(c.Entry)
	if e != nil {
		return c, e
	}
	if !st.Mode().IsRegular() {
		return c, fmt.Errorf("entry is not a regular scene")
	}
	// Do NOT compare source files to downloaded hashes here: they are editable.
	return c, nil
}
func (s *Store) OpenRecent(source string) (Copy, error) {
	if _, e := httpsURL(source); e != nil {
		return Copy{}, e
	}
	copies, e := s.Copies()
	if e != nil {
		return Copy{}, e
	}
	for _, c := range copies {
		if c.ManifestURL == source {
			return c, nil
		}
	}
	return Copy{}, os.ErrNotExist
}

type Update struct {
	Available bool     `json:"available"`
	Current   string   `json:"current"`
	Remote    string   `json:"remote"`
	Manifest  Manifest `json:"manifest"`
}

func (c *Client) CheckUpdate(ctx context.Context, local Copy, approved string) (Update, error) {
	m, e := c.FetchManifest(ctx, local.ManifestURL, approved)
	if e != nil {
		return Update{}, e
	}
	if m.ExampleID != local.Manifest.ExampleID || m.Origin != local.Manifest.Origin {
		return Update{}, fmt.Errorf("remote example identity changed")
	}
	a, e := version(local.Manifest.Version)
	if e != nil {
		return Update{}, e
	}
	b, e := version(m.Version)
	if e != nil {
		return Update{}, e
	}
	if a == b && m.Archive.SHA256 != local.Manifest.Archive.SHA256 {
		return Update{}, fmt.Errorf("published version changed its content hash")
	}
	return Update{Available: less(a, b), Current: local.Manifest.Version, Remote: m.Version, Manifest: m}, nil
}
