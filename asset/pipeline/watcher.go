package pipeline

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ChangeKind uint8

const (
	ChangeWrite ChangeKind = iota
	ChangeRemove
)

type Change struct {
	Path string
	Kind ChangeKind
}

type Watcher interface {
	Start(root string, skip []string, interval time.Duration, fn func([]Change)) (func(), error)
}

type fileState struct {
	size  int64
	mtime int64
}

type PollWatcher struct{}

func (PollWatcher) Start(root string, skip []string, interval time.Duration, fn func([]Change)) (func(), error) {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	snapshot, err := scanTree(root, skip)
	if err != nil {
		return nil, err
	}
	stop := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
			next, err := scanTree(root, skip)
			if err != nil {
				continue
			}
			changes := diffTrees(snapshot, next)
			snapshot = next
			if len(changes) > 0 {
				fn(changes)
			}
		}
	}()
	return func() { once.Do(func() { close(stop) }) }, nil
}

func scanTree(root string, skip []string) (map[string]fileState, error) {
	out := map[string]fileState{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			for _, s := range skip {
				if name == s {
					return filepath.SkipDir
				}
			}
			if len(name) > 0 && name[0] == '.' {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out[filepath.ToSlash(rel)] = fileState{size: info.Size(), mtime: info.ModTime().UnixNano()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func diffTrees(before, after map[string]fileState) []Change {
	var changes []Change
	for path, state := range after {
		prev, ok := before[path]
		if !ok || prev != state {
			changes = append(changes, Change{Path: path, Kind: ChangeWrite})
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			changes = append(changes, Change{Path: path, Kind: ChangeRemove})
		}
	}
	return changes
}
