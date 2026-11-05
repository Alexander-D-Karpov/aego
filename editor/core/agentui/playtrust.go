package agentui

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PlaytestTrust is a user-local permission for one project and playtest kind.
type PlaytestTrust struct {
	dir, project, kind string
}

func NewPlaytestTrust(dir, project string, native bool) *PlaytestTrust {
	if dir == "" || project == "" {
		return nil
	}
	root, err := filepath.Abs(project)
	if err != nil {
		return nil
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil
	}
	kind := "scripts"
	if native {
		kind = "native"
	}
	return &PlaytestTrust{dir: filepath.Join(dir, "agent-playtests"), project: filepath.Clean(root), kind: kind}
}

func (t *PlaytestTrust) path() string {
	sum := sha256.Sum256([]byte(t.project + "\x00" + t.kind))
	return filepath.Join(t.dir, fmt.Sprintf("%x.json", sum))
}

type playtestPermission struct {
	Version int    `json:"version"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
}

func (t *PlaytestTrust) Allowed() bool {
	if t == nil {
		return false
	}
	info, err := os.Lstat(t.path())
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<10 {
		return false
	}
	data, err := os.ReadFile(t.path())
	var p playtestPermission
	return err == nil && json.Unmarshal(data, &p) == nil && p.Version == 1 && p.Project == t.project && p.Kind == t.kind
}

func (t *PlaytestTrust) Allow() error {
	if t == nil {
		return errors.New("cannot save the playtest permission without a user configuration directory")
	}
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(playtestPermission{Version: 1, Project: t.project, Kind: t.kind})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(t.dir, ".permission-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, t.path())
}

func (t *PlaytestTrust) Revoke() error {
	if t == nil {
		return nil
	}
	err := os.Remove(t.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
