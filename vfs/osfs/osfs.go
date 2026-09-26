package osfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"aego/core/errs"
	"aego/core/log"
)

type Dir struct {
	Root string
}

type File struct {
	f    *os.File
	size int64
}

func (f *File) Read(p []byte) (int, error) {
	return f.f.Read(p)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *File) Close() error {
	return f.f.Close()
}

func (f *File) Size() int64 {
	return f.size
}

func (d Dir) Full(name string) string {
	return filepath.Join(d.Root, filepath.FromSlash(name))
}

func (d Dir) Open(name string) (*File, error) {
	f, err := os.Open(d.Full(name))
	if err != nil {
		return nil, mapErr(err, name)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, mapErr(err, name)
	}
	if st.IsDir() {
		f.Close()
		return nil, errs.New(errs.VFSNotFound, "path is a directory", log.F("path", name))
	}
	return &File{f: f, size: st.Size()}, nil
}

func (d Dir) Exists(name string) bool {
	_, err := os.Stat(d.Full(name))
	return err == nil
}

func (d Dir) WriteFile(name string, data []byte) error {
	full := d.Full(name)
	dir := filepath.Dir(full)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return mapErr(err, name)
	}
	tmp, err := os.CreateTemp(dir, ".aego-*.tmp")
	if err != nil {
		return mapErr(err, name)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return mapErr(err, name)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return mapErr(err, name)
	}
	if err := os.Rename(tmpName, full); err != nil {
		os.Remove(tmpName)
		return mapErr(err, name)
	}
	return nil
}

func mapErr(err error, name string) error {
	if errors.Is(err, fs.ErrNotExist) {
		return errs.Wrap(err, errs.VFSNotFound, "file not found").With(log.F("path", name))
	}
	return errs.Wrap(err, errs.VFSIO, "file system error").With(log.F("path", name))
}
