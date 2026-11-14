package playground

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"aego/platform"
)

// delayedSaveFS exposes the save bytes but holds the worker before Write
// returns. Loading can then finish before the save-completion callback is
// queued. All storage is private to the test.
type delayedSaveFS struct {
	mu      sync.RWMutex
	files   map[string][]byte
	blocked atomic.Bool
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func newDelayedSaveFS() *delayedSaveFS {
	return &delayedSaveFS{files: make(map[string][]byte), entered: make(chan struct{}), gate: make(chan struct{})}
}

func (f *delayedSaveFS) release() { f.once.Do(func() { close(f.gate) }) }

func (f *delayedSaveFS) ReadFile(name string) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, ok := f.files[path.Clean(name)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (f *delayedSaveFS) WriteFile(name string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[path.Clean(name)] = append([]byte(nil), data...)
	return nil
}

func (f *delayedSaveFS) Remove(name string) error {
	name = path.Clean(name)
	if strings.HasSuffix(name, ".png") && f.blocked.CompareAndSwap(false, true) {
		close(f.entered)
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, name)
	return nil
}

func (f *delayedSaveFS) Stat(name string) (platform.UserEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, ok := f.files[path.Clean(name)]
	if !ok {
		return platform.UserEntry{}, fs.ErrNotExist
	}
	return platform.UserEntry{Name: path.Base(name), Size: int64(len(data))}, nil
}

func (f *delayedSaveFS) List(dir string) ([]platform.UserEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	prefix := path.Clean(dir) + "/"
	var entries []platform.UserEntry
	for name, data := range f.files {
		if rest, ok := strings.CutPrefix(name, prefix); ok && !strings.Contains(rest, "/") {
			entries = append(entries, platform.UserEntry{Name: rest, Size: int64(len(data))})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}
