//go:build !js

package update

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"aego/engine/module"
	upd "aego/update"
)

func newBackend() backend {
	return &native{}
}

// native updates the game's own install from its release server.
type native struct {
	rel  *upd.Release
	file string
	in   upd.Install
}

func (n *native) init(m *Module, ctx *module.InitContext) error {
	descs := Settings()
	var err error
	if m.mode, err = ctx.Setting(descs[0]); err != nil {
		return err
	}
	if m.ch, err = ctx.Setting(descs[1]); err != nil {
		return err
	}
	if exe, err := os.Executable(); err == nil {
		n.in = upd.DetectGame(exe)
	}
	upd.Clean(downloads(m), m.ver)
	if policy(m) != upd.Off {
		n.check(m)
	}
	return nil
}

func policy(m *Module) upd.Mode {
	if m.st == nil || m.mode == 0 {
		return upd.Auto
	}
	return upd.ParseMode(m.st.ChoiceName(m.mode))
}

func channel(m *Module) upd.Channel {
	if m.st == nil || m.ch == 0 {
		return upd.Stable
	}
	return upd.ParseChannel(m.st.ChoiceName(m.ch))
}

// downloads is where the game's updates are fetched to: UserCacheDir/<game>/updates.
func downloads(m *Module) string {
	dir, _ := os.UserCacheDir()
	return filepath.Join(dir, m.game, "updates")
}

func (n *native) check(m *Module) error {
	m.mu.Lock()
	if !m.Configured() || m.phase == Checking || m.phase == Downloading {
		m.mu.Unlock()
		return nil
	}
	m.phase, m.err = Checking, ""
	src := &upd.Source{BaseURL: m.sec.URL, Channel: channel(m), Key: m.sec.PublicKey}
	m.mu.Unlock()
	go func() {
		r, err := src.Check(context.Background())
		m.mu.Lock()
		switch {
		case err != nil:
			m.phase, m.err = Failed, err.Error()
		case !r.Newer(upd.Build{Version: m.ver}):
			m.phase = UpToDate
		default:
			m.phase, n.rel, m.newVer, m.notes = Available, r, r.Version, r.Notes
		}
		auto := m.phase == Available && policy(m) == upd.Auto
		m.mu.Unlock()
		if auto {
			n.download(m)
		}
	}()
	return nil
}

func (n *native) download(m *Module) {
	m.mu.Lock()
	r := n.rel
	if r == nil || m.phase == Downloading {
		m.mu.Unlock()
		return
	}
	m.phase = Downloading
	m.mu.Unlock()
	go func() {
		kind := upd.KindTarball
		if n.in.Kind == upd.KindAppImage || n.in.Kind == upd.KindApp {
			kind = n.in.Kind
		}
		var file string
		a, err := r.Asset(runtime.GOOS, runtime.GOARCH, kind)
		if err == nil {
			file, err = upd.Download(context.Background(), nil, a, filepath.Join(downloads(m), r.Version), nil)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if err != nil {
			m.phase, m.err = Failed, err.Error()
			return
		}
		m.phase, n.file = Ready, file
	}()
}

func (n *native) apply(m *Module) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != Ready || n.rel == nil {
		return errors.New("no update is downloaded")
	}
	if _, err := n.in.Apply(n.file, n.rel.Version); err != nil {
		return err
	}
	cmd := exec.Command(n.in.Launch, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	if m.eng != nil {
		m.eng.Quit()
	}
	return nil
}
