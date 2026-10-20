package memfs

import (
	"bytes"

	"aego/core/errs"
	"aego/core/log"
	"aego/vfs"
)

type Source struct {
	files map[string][]byte
}

type file struct {
	*bytes.Reader
}

func (file) Close() error {
	return nil
}

func New(files map[string][]byte) *Source {
	return &Source{files: files}
}

func (s *Source) Open(name string) (vfs.File, error) {
	data, ok := s.files[name]
	if !ok {
		return nil, errs.New(errs.VFSNotFound, "file is not in the game data", log.F("path", name))
	}
	return file{bytes.NewReader(data)}, nil
}

func (s *Source) Exists(name string) bool {
	_, ok := s.files[name]
	return ok
}

func (s *Source) WriteFile(name string, _ []byte) error {
	return errs.New(errs.VFSReadOnly, "game data is read-only", log.F("path", name))
}

func (s *Source) Len() int {
	return len(s.files)
}

var _ vfs.Source = (*Source)(nil)
