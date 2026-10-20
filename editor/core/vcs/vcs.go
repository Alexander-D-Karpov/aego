package vcs

import (
	"os"
	"time"
)

type FileStatus uint8

const (
	Unmodified FileStatus = iota
	Modified
	Added
	Deleted
	Renamed
	Untracked
	Conflicted
)

var statusNames = [...]string{"unmodified", "modified", "added", "deleted", "renamed", "untracked", "conflicted"}

func (s FileStatus) String() string {
	return statusNames[s]
}

type Commit struct {
	Hash    string
	Short   string
	Author  string
	When    time.Time
	Subject string
}

type Provider interface {
	Name() string
	Root() string
	Show(rev, rel string) ([]byte, error)
	Status() (map[string]FileStatus, error)
	Log(rel string, n int) ([]Commit, error)
}

func Detect(projectRoot string) (Provider, bool) {
	if g, ok := openGit(projectRoot); ok {
		return g, true
	}
	return nil, false
}

func ReadWorking(abs string) ([]byte, error) {
	data, err := os.ReadFile(abs)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}
