package platform

import "time"

type UserEntry struct {
	Name string
	Size int64
	Mod  time.Time
	Dir  bool
}

type UserFS interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	Remove(path string) error
	Stat(path string) (UserEntry, error)
	List(dir string) ([]UserEntry, error)
}
