package res

import (
	"bytes"
	"embed"
	"io"
	"io/fs"

	"aego/core/errs"
	"aego/core/log"
	"aego/vfs"
)

//go:embed shaders
//go:embed fonts
var files embed.FS

type source struct{}

type file struct {
	*bytes.Reader
	size int64
}

func (f *file) Close() error {
	return nil
}

func (f *file) Size() int64 {
	return f.size
}

func Source() vfs.Source {
	return source{}
}

func FS() fs.FS {
	return files
}

func (source) Open(name string) (vfs.File, error) {
	data, err := files.ReadFile(name)
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSNotFound, "embedded file not found").With(log.F("path", name))
	}
	return &file{Reader: bytes.NewReader(data), size: int64(len(data))}, nil
}

func (source) Exists(name string) bool {
	f, err := files.Open(name)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func (source) WriteFile(name string, data []byte) error {
	return errs.New(errs.VFSReadOnly, "engine resources are read-only", log.F("path", name))
}

var _ io.ReadSeekCloser = (*file)(nil)
