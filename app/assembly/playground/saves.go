package playground

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"aego/contracts/persistence"
	"aego/engine"
	"aego/platform"
	"aego/save"
)

// UseMemorySaves gives e a save store kept in memory, for runs in a browser page that must not touch the visitor's storage.
func UseMemorySaves(e *engine.Engine) {
	e.SetService(persistence.StoreID, save.NewStoreFS("saves", &memUser{files: map[string][]byte{}}))
}

type memUser struct {
	files map[string][]byte
}

func (m *memUser) ReadFile(p string) ([]byte, error) {
	data, ok := m.files[path.Clean(p)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (m *memUser) WriteFile(p string, data []byte) error {
	m.files[path.Clean(p)] = append([]byte(nil), data...)
	return nil
}

func (m *memUser) Remove(p string) error {
	delete(m.files, path.Clean(p))
	return nil
}

func (m *memUser) Stat(p string) (platform.UserEntry, error) {
	data, ok := m.files[path.Clean(p)]
	if !ok {
		return platform.UserEntry{}, fs.ErrNotExist
	}
	return platform.UserEntry{Name: path.Base(p), Size: int64(len(data)), Mod: time.Time{}}, nil
}

func (m *memUser) List(dir string) ([]platform.UserEntry, error) {
	prefix := path.Clean(dir) + "/"
	var out []platform.UserEntry
	for p, data := range m.files {
		if rest, ok := strings.CutPrefix(p, prefix); ok && !strings.Contains(rest, "/") {
			out = append(out, platform.UserEntry{Name: rest, Size: int64(len(data))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
