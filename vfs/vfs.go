package vfs

import (
	"io"
	"path"
	"slices"
	"strings"
	"sync"

	"aego/core/errs"
	"aego/core/log"
	"aego/vfs/osfs"
)

type File interface {
	io.ReadSeekCloser
	Size() int64
}

type Source interface {
	Open(name string) (File, error)
	Exists(name string) bool
	WriteFile(name string, data []byte) error
}

type FS interface {
	Open(path string) (File, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Exists(path string) bool
}

type Mount struct {
	Scheme   string
	Root     string
	ReadOnly bool
	Source   Source
}

type VFS struct {
	mu     sync.RWMutex
	mounts map[string][]Mount
}

var overlaySchemes = map[string]bool{"res": true, "engine": true}

func New(mounts ...Mount) (*VFS, error) {
	v := &VFS{mounts: make(map[string][]Mount, len(mounts))}
	for _, m := range mounts {
		if err := v.Mount(m); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (v *VFS) Mount(m Mount) error {
	if m.Scheme == "" {
		return errs.New(errs.VFSMount, "mount needs a scheme")
	}
	if m.Source == nil {
		m.Source = dirSource{dir: osfs.Dir{Root: m.Root}}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	existing := v.mounts[m.Scheme]
	if len(existing) > 0 && !overlaySchemes[m.Scheme] {
		return errs.New(errs.VFSMount, "scheme does not support overlay mounts",
			log.F("scheme", m.Scheme), log.F("root", m.Root))
	}
	v.mounts[m.Scheme] = append(slices.Clip(existing), m)
	return nil
}

func (v *VFS) Unmount(scheme string) {
	v.mu.Lock()
	delete(v.mounts, scheme)
	v.mu.Unlock()
}

func (v *VFS) Schemes() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	out := make([]string, 0, len(v.mounts))
	for s := range v.mounts {
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func Split(p string) (scheme, rel string, err error) {
	i := strings.Index(p, "://")
	if i <= 0 {
		return "", "", errs.New(errs.VFSBadPath, "path must be scheme://path", log.F("path", p))
	}
	scheme = p[:i]
	rest := p[i+3:]
	if strings.ContainsRune(rest, '\\') || strings.ContainsRune(rest, 0) {
		return "", "", errs.New(errs.VFSBadPath, "path contains forbidden characters", log.F("path", p))
	}
	rel = strings.TrimPrefix(path.Clean("/"+rest), "/")
	return scheme, rel, nil
}

func (v *VFS) lookup(p string) (string, []Mount, error) {
	scheme, rel, err := Split(p)
	if err != nil {
		return "", nil, err
	}
	if rel == "" {
		return "", nil, errs.New(errs.VFSBadPath, "empty path", log.F("path", p))
	}
	v.mu.RLock()
	mounts := v.mounts[scheme]
	v.mu.RUnlock()
	if len(mounts) == 0 {
		return "", nil, errs.New(errs.VFSUnknownScheme, "no mount for scheme", log.F("scheme", scheme), log.F("path", p))
	}
	return rel, mounts, nil
}

func (v *VFS) Open(p string) (File, error) {
	rel, mounts, err := v.lookup(p)
	if err != nil {
		return nil, err
	}
	for i := len(mounts) - 1; i >= 0; i-- {
		f, err := mounts[i].Source.Open(rel)
		if err == nil {
			return f, nil
		}
		if !errs.Has(err, errs.VFSNotFound) {
			return nil, err
		}
	}
	return nil, errs.New(errs.VFSNotFound, "file not found", log.F("path", p))
}

func (v *VFS) ReadFile(p string) ([]byte, error) {
	f, err := v.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, f.Size())
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "read failed").With(log.F("path", p))
	}
	return buf, nil
}

func (v *VFS) WriteFile(p string, data []byte) error {
	rel, mounts, err := v.lookup(p)
	if err != nil {
		return err
	}
	for i := len(mounts) - 1; i >= 0; i-- {
		if mounts[i].ReadOnly {
			continue
		}
		return mounts[i].Source.WriteFile(rel, data)
	}
	return errs.New(errs.VFSReadOnly, "no writable mount for path", log.F("path", p))
}

func (v *VFS) Exists(p string) bool {
	rel, mounts, err := v.lookup(p)
	if err != nil {
		return false
	}
	for i := len(mounts) - 1; i >= 0; i-- {
		if mounts[i].Source.Exists(rel) {
			return true
		}
	}
	return false
}

type dirSource struct {
	dir osfs.Dir
}

func (s dirSource) Open(name string) (File, error) {
	f, err := s.dir.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s dirSource) Exists(name string) bool {
	return s.dir.Exists(name)
}

func (s dirSource) WriteFile(name string, data []byte) error {
	return s.dir.WriteFile(name, data)
}
