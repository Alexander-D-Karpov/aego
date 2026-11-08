package pipeline

import (
	"errors"
	"io"
	"math"
	"os"
)

// ErrExternalFileLimit indicates a special file or an input over budget.
var ErrExternalFileLimit = errors.New("external file is not regular or exceeds the size limit")

// ReadExternalFile reads a user-selected input with a strict byte budget.
// Check before opening as well as after opening so a named pipe is not normally
// opened as though it were an ordinary document. Symlinks are followed because
// the caller explicitly selected the source; project writes use separate guards.
func ReadExternalFile(name string, limit int64) ([]byte, error) {
	if limit <= 0 || limit == math.MaxInt64 {
		return nil, errors.New("invalid external file size limit")
	}
	check := func(info os.FileInfo) error {
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
			return ErrExternalFileLimit
		}
		return nil
	}
	info, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if err := check(info); err != nil {
		return nil, err
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if err := check(info); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrExternalFileLimit
	}
	return data, nil
}
