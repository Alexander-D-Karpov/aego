package session

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aego/editor/core/document"
	"aego/editor/core/state"
)

const (
	Dir           = ".aego/session"
	ExitRestart   = 3
	EnvSupervised = "AEGO_SUPERVISED"
	stateFile     = "handoff.aetext"
	readyFile     = "handoff.ready"
	scenesDir     = "scenes"
	restartFile   = "restart"
	statusFile    = "build.status"
)

const (
	StateBuilding = "building"
	StateOK       = "ok"
	StateError    = "error"
)

type Handoff struct {
	State  *state.Editor
	Scenes map[string][]byte
}

type Status struct {
	State string
	OK    bool
	Time  time.Time
	Log   string
}

func file(root string, parts ...string) string {
	return filepath.Join(append([]string{root, filepath.FromSlash(Dir)}, parts...)...)
}

func discard(root string) {
	os.Remove(file(root, readyFile))
	os.Remove(file(root, stateFile))
	os.RemoveAll(file(root, scenesDir))
}

func Write(root string, h *Handoff) error {
	discard(root)
	if err := document.WriteAtomic(file(root, stateFile), state.Encode(h.State)); err != nil {
		return err
	}
	for rel, data := range h.Scenes {
		name := hex.EncodeToString([]byte(rel)) + document.SceneExt
		if err := document.WriteAtomic(file(root, scenesDir, name), data); err != nil {
			return err
		}
	}
	ClearRestart(root)
	return document.WriteAtomic(file(root, readyFile), []byte(time.Now().Format(time.RFC3339Nano)))
}

func Pending(root string) bool {
	_, err := os.Stat(file(root, readyFile))
	return err == nil
}

func Take(root string) (*Handoff, bool) {
	if !Pending(root) {
		return nil, false
	}
	defer discard(root)
	data, err := os.ReadFile(file(root, stateFile))
	if err != nil {
		return nil, false
	}
	st, err := state.Decode(data, stateFile)
	if err != nil {
		return nil, false
	}
	h := &Handoff{State: st, Scenes: map[string][]byte{}}
	entries, _ := os.ReadDir(file(root, scenesDir))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, document.SceneExt) {
			continue
		}
		raw, err := hex.DecodeString(strings.TrimSuffix(name, document.SceneExt))
		if err != nil {
			continue
		}
		blob, err := os.ReadFile(file(root, scenesDir, name))
		if err != nil {
			continue
		}
		h.Scenes[string(raw)] = blob
	}
	return h, true
}

func Clear(root string) {
	os.RemoveAll(file(root))
}

func RequestRestart(root string) error {
	return document.WriteAtomic(file(root, restartFile), nil)
}

func RestartRequested(root string) bool {
	_, err := os.Stat(file(root, restartFile))
	return err == nil
}

func ClearRestart(root string) {
	os.Remove(file(root, restartFile))
}

func WriteState(root, state, output string) error {
	data := state + "\n" + time.Now().Format(time.RFC3339Nano) + "\n" + output
	return document.WriteAtomic(file(root, statusFile), []byte(data))
}

func WriteStatus(root string, ok bool, output string) error {
	if ok {
		return WriteState(root, StateOK, output)
	}
	return WriteState(root, StateError, output)
}

func ReadStatus(root string) (Status, bool) {
	data, err := os.ReadFile(file(root, statusFile))
	if err != nil {
		return Status{}, false
	}
	parts := strings.SplitN(string(data), "\n", 3)
	if len(parts) < 2 {
		return Status{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return Status{}, false
	}
	st := Status{State: parts[0], OK: parts[0] == StateOK, Time: t}
	if len(parts) == 3 {
		st.Log = parts[2]
	}
	return st, true
}
