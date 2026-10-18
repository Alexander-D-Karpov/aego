package save

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aego/core/errs"
	"aego/core/log"
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
	dir string
}

func NewStore(dir string) *Store {
	return &Store{dir: dir}
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

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".save-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

func (s *Store) Write(slot string, data, preview []byte) error {
	path, err := s.Path(slot)
	if err != nil {
		return err
	}
	if err := writeAtomic(path, data); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the save").With(log.F("slot", slot))
	}
	png := strings.TrimSuffix(path, Ext) + PreviewExt
	if len(preview) == 0 {
		os.Remove(png)
		return nil
	}
	if err := writeAtomic(png, preview); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write the save preview").With(log.F("slot", slot))
	}
	return nil
}

func (s *Store) Read(slot string) ([]byte, error) {
	path, err := s.Path(slot)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
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
	_, err = os.Stat(path)
	return err == nil
}

func (s *Store) Delete(slot string) error {
	path, err := s.Path(slot)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return errs.Wrap(err, errs.VFSIO, "cannot delete the save").With(log.F("slot", slot))
	}
	os.Remove(strings.TrimSuffix(path, Ext) + PreviewExt)
	return nil
}

func (s *Store) List() []SlotInfo {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var out []SlotInfo
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, Ext) {
			continue
		}
		slot := strings.TrimSuffix(name, Ext)
		path := filepath.Join(s.dir, name)
		info := SlotInfo{Slot: slot, Path: path}
		if st, err := e.Info(); err == nil {
			info.Size, info.Mod = st.Size(), st.ModTime()
		}
		if data, err := os.ReadFile(path); err != nil {
			info.Err = err
		} else if d, err := Decode(data, name); err != nil {
			info.Err = err
		} else {
			info.Meta = d.Meta
		}
		if png := filepath.Join(s.dir, slot+PreviewExt); fileExists(png) {
			info.Preview = png
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mod.After(out[j].Mod) })
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
