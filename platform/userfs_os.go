//go:build !js

package platform

import (
	"os"
	"path/filepath"
)

type osFiles struct{}

func UserFiles() UserFS {
	return osFiles{}
}

func configBase() (string, error) {
	return os.UserConfigDir()
}

func (osFiles) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (osFiles) WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".aego-*")
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

func (osFiles) Remove(path string) error {
	return os.Remove(path)
}

func entryOf(name string, info os.FileInfo) UserEntry {
	return UserEntry{Name: name, Size: info.Size(), Mod: info.ModTime(), Dir: info.IsDir()}
}

func (osFiles) Stat(path string) (UserEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return UserEntry{}, err
	}
	return entryOf(filepath.Base(path), info), nil
}

func (osFiles) List(dir string) ([]UserEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]UserEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, entryOf(e.Name(), info))
	}
	return out, nil
}
