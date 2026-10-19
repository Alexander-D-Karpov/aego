package save

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aego/core/errs"
	"aego/core/log"
	"aego/platform"
)

type SlotInfo struct {
	Slot    string
	Path    string
	Size    int64
	Mod     time.Time
	Meta    Meta
	Preview string
	Err     error
}

type Store struct {
	dir   string
	files platform.UserFS
}

func NewStore(dir string) *Store {
	return NewStoreFS(dir, platform.UserFiles())
}

func NewStoreFS(dir string, files platform.UserFS) *Store {
	return &Store{dir: dir, files: files}
}

func ValidSlot(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) Path(slot string) (string, error) {
	if !ValidSlot(slot) {
		return "", errs.New(errs.PlatformConfig, "slot name may use letters, digits, - and _", log.F("slot", slot))
	}
	return filepath.Join(s.dir, slot+Ext), nil
}

func previewOf(path string) string {
	return strings.TrimSuffix(path, Ext) + PreviewExt
}

func (s *Store) Write(slot string, data, preview []byte) error {
	path, err := s.Path(slot)
	if err != nil {
		return err
	}
	if err := s.files.WriteFile(path, data); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the save").With(log.F("slot", slot))
	}
	png := previewOf(path)
	if len(preview) == 0 {
		s.files.Remove(png)
		return nil
	}
	if err := s.files.WriteFile(png, preview); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the save preview").With(log.F("slot", slot))
	}
	return nil
}

func (s *Store) Read(slot string) ([]byte, error) {
	path, err := s.Path(slot)
	if err != nil {
		return nil, err
	}
	data, err := s.files.ReadFile(path)
	if err != nil {
		return nil, errs.Wrap(err, errs.VFSIO, "cannot read the save").With(log.F("slot", slot))
	}
	return data, nil
}

func (s *Store) Load(slot string) (*Doc, error) {
	data, err := s.Read(slot)
	if err != nil {
		return nil, err
	}
	return Decode(data, slot+Ext)
}

func (s *Store) Exists(slot string) bool {
	path, err := s.Path(slot)
	if err != nil {
		return false
	}
	_, err = s.files.Stat(path)
	return err == nil
}

func (s *Store) Delete(slot string) error {
	path, err := s.Path(slot)
	if err != nil {
		return err
	}
	if err := s.files.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errs.Wrap(err, errs.VFSIO, "cannot delete the save").With(log.F("slot", slot))
	}
	s.files.Remove(previewOf(path))
	return nil
}

func (s *Store) List() []SlotInfo {
	entries, err := s.files.List(s.dir)
	if err != nil {
		return nil
	}
	var out []SlotInfo
	for _, e := range entries {
		if e.Dir || !strings.HasSuffix(e.Name, Ext) {
			continue
		}
		slot := strings.TrimSuffix(e.Name, Ext)
		path := filepath.Join(s.dir, e.Name)
		info := SlotInfo{Slot: slot, Path: path, Size: e.Size, Mod: e.Mod}
		if data, err := s.files.ReadFile(path); err != nil {
			info.Err = err
		} else if d, err := Decode(data, e.Name); err != nil {
			info.Err = err
		} else {
			info.Meta = d.Meta
		}
		if _, err := s.files.Stat(previewOf(path)); err == nil {
			info.Preview = previewOf(path)
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mod.After(out[j].Mod) })
	return out
}
