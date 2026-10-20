package exportfs

import (
	"aego/core/errs"
	"aego/core/log"
	"aego/vfs"
	"aego/vfs/osfs"
)

type Source struct {
	dir   osfs.Dir
	files map[string]int64
}

func New(root string, files map[string]int64) *Source {
	return &Source{dir: osfs.Dir{Root: root}, files: files}
}

func (s *Source) Open(name string) (vfs.File, error) {
	size, ok := s.files[name]
	if !ok {
		return nil, errs.New(errs.VFSNotFound, "file is not listed in the export manifest", log.F("path", name))
	}
	f, err := s.dir.Open(name)
	if err != nil {
		return nil, err
	}
	if f.Size() != size {
		f.Close()
		return nil, errs.New(errs.VFSIO, "file size differs from the export manifest",
			log.F("path", name), log.F("size", f.Size()), log.F("want", size))
	}
	return f, nil
}

func (s *Source) Exists(name string) bool {
	_, ok := s.files[name]
	return ok && s.dir.Exists(name)
}

func (s *Source) WriteFile(name string, _ []byte) error {
	return errs.New(errs.VFSReadOnly, "exported game data is read-only", log.F("path", name))
}

func (s *Source) Len() int {
	return len(s.files)
}

var _ vfs.Source = (*Source)(nil)
