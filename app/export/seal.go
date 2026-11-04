package export

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Agent launches verify all executable, data and web files, not only the manifest.
// A private copy pins the exact bytes while the game or loopback server uses them.
func Seal(dir string) ([32]byte, error) { return walkExport(dir, nil) }

func VerifiedCopy(dir string, want [32]byte) (string, error) {
	tmp, err := os.MkdirTemp("", "aego-export-")
	if err != nil {
		return "", err
	}
	target, err := os.OpenRoot(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	got, err := walkExport(dir, target)
	target.Close()
	if err == nil && got != want {
		err = errors.New("export contents changed; export again before launch")
	}
	if err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return tmp, nil
}

func walkExport(dir string, dst *os.Root) ([32]byte, error) {
	var sum [32]byte
	clean, err := filepath.Abs(dir)
	if err != nil {
		return sum, err
	}
	actual, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return sum, err
	}
	if actual != clean {
		return sum, errors.New("export directory was replaced by a symlink")
	}
	root, err := os.OpenRoot(clean)
	if err != nil {
		return sum, err
	}
	defer root.Close()
	const maxFiles = 32768
	const maxBytes = 1 << 30
	var count int
	var total int64
	h := sha256.New()
	buf := make([]byte, 64<<10)
	err = fs.WalkDir(root.FS(), ".", func(name string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		count++
		if count > maxFiles {
			return errors.New("agent export exceeds file limit")
		}
		rel := filepath.FromSlash(name)
		mode := de.Type()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in export: %s", name)
		}
		if de.IsDir() {
			if dst != nil {
				return dst.Mkdir(rel, 0700)
			}
			return nil
		}
		if !mode.IsRegular() {
			return fmt.Errorf("non-regular export file: %s", name)
		}
		f, err := root.Open(rel)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxBytes-total {
			return errors.New("invalid file or export exceeds 1 GiB agent launch limit")
		}
		total += info.Size()
		var header [16]byte
		binary.LittleEndian.PutUint32(header[:4], uint32(len(name)))
		binary.LittleEndian.PutUint32(header[4:8], uint32(info.Mode().Perm()&0111))
		binary.LittleEndian.PutUint64(header[8:], uint64(info.Size()))
		h.Write(header[:])
		io.WriteString(h, name)
		var writer io.Writer = h
		var out *os.File
		if dst != nil {
			out, err = dst.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600|info.Mode().Perm()&0111)
			if err != nil {
				return err
			}
			writer = io.MultiWriter(h, out)
		}
		n, err := io.CopyBuffer(writer, io.LimitReader(f, info.Size()+1), buf)
		if out != nil {
			if e := out.Close(); err == nil {
				err = e
			}
		}
		if err != nil {
			return err
		}
		if n != info.Size() {
			return errors.New("export changed during verification")
		}
		return nil
	})
	if err == nil {
		copy(sum[:], h.Sum(nil))
	}
	return sum, err
}
