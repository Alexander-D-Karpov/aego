package edit

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"time"

	"aego/app/updates"
	"aego/editor/core/prefs"
	"aego/editor/core/updateui"
	"aego/update"
)

// DownloadPage is where a copy that cannot update itself gets a new version.
const DownloadPage = "https://aego.akarpov.ru/install.html"

// updateManager checks, downloads and installs Aego's updates for the launcher and the editor of this process (D362).
type updateManager struct {
	mu      sync.Mutex
	dir     string
	pol     update.Policy
	st      updateui.State
	rel     *update.Release
	file    string
	in      update.Install
	inErr   error
	started bool
	busy    bool
}

var (
	updatesOnce sync.Once
	theUpdates  *updateManager
	atExit      *exec.Cmd
)

// updatesFor is the one update service of the process; dir holds prefs.aetext with the policy.
func updatesFor(dir string) *updateManager {
	updatesOnce.Do(func() {
		m := &updateManager{dir: dir, pol: update.Policy{Mode: update.Auto, Channel: update.Stable}}
		if p, err := prefs.Load(dir); err == nil {
			m.pol = p.Updates
		}
		m.in, m.inErr = updates.Installed()
		m.st.Installable = m.inErr == nil
		m.st.RolledBack = updates.RolledBack()
		theUpdates = m
	})
	return theUpdates
}

// runAtExit starts what an update left for the editor's exit: the Windows installer, which restarts the editor itself.
func runAtExit() {
	if atExit != nil {
		atExit.Start()
		atExit = nil
	}
}

func (m *updateManager) State() updateui.State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

func (m *updateManager) Policy() update.Policy {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pol
}

// save writes the policy into prefs.aetext without touching the rest of it; the caller holds mu.
func (m *updateManager) save() {
	if m.dir == "" {
		return
	}
	p, err := prefs.Load(m.dir)
	if err != nil {
		return
	}
	p.Updates = m.pol
	_ = prefs.Save(m.dir, p)
}

// Tick checks at start and every six hours of a long session, unless updates are off.
func (m *updateManager) Tick(now time.Time) {
	m.mu.Lock()
	due := !m.busy && m.pol.Due(now, m.started)
	m.started = true
	m.mu.Unlock()
	if due {
		m.Check()
	}
}

func (m *updateManager) Check() {
	m.mu.Lock()
	if m.busy {
		m.mu.Unlock()
		return
	}
	m.busy = true
	m.st.Phase, m.st.Err = updateui.Checking, ""
	ch := m.pol.Channel
	m.mu.Unlock()
	go func() {
		r, err := updates.Source(ch).Check(context.Background())
		m.mu.Lock()
		m.busy = false
		m.pol.LastCheck = time.Now()
		m.save()
		switch {
		case err != nil:
			m.st.Phase, m.st.Err = updateui.Failed, err.Error()
		case !m.pol.Wants(r, updates.Build()):
			m.st.Phase = updateui.UpToDate
		default:
			m.rel = r
			m.st.Phase, m.st.Version, m.st.Notes = updateui.Available, r.Version, r.Notes
		}
		auto := m.st.Phase == updateui.Available && m.pol.Mode == update.Auto && m.inErr == nil
		m.mu.Unlock()
		if auto {
			m.Download()
		}
	}()
}

func (m *updateManager) Download() {
	m.mu.Lock()
	r := m.rel
	if r == nil || m.busy || m.inErr != nil {
		m.mu.Unlock()
		return
	}
	m.busy = true
	m.st.Phase, m.st.Done, m.st.Total = updateui.Downloading, 0, 0
	m.mu.Unlock()
	go func() {
		file, err := updates.Fetch(context.Background(), r, m.in, func(done, total int64) {
			m.mu.Lock()
			m.st.Done, m.st.Total = done, total
			m.mu.Unlock()
		})
		m.mu.Lock()
		defer m.mu.Unlock()
		m.busy = false
		if err != nil {
			m.st.Phase, m.st.Err = updateui.Failed, err.Error()
			return
		}
		m.file = file
		m.st.Phase = updateui.Ready
	}()
}

func (m *updateManager) Install() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inErr != nil {
		return false, updates.ErrNotInstalled
	}
	if m.rel == nil || m.file == "" {
		return false, update.ErrNoAsset
	}
	a, err := updates.Apply(m.in, m.file, m.rel)
	if err != nil {
		return false, err
	}
	if a.Exit != nil {
		atExit = a.Exit
		return false, nil
	}
	os.Setenv(EnvEditorExe, m.in.Launch)
	return true, nil
}

func (m *updateManager) Skip() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rel != nil {
		m.pol.Skip(m.rel.Version)
		m.save()
	}
	m.st.Phase, m.rel = updateui.UpToDate, nil
}

func (m *updateManager) Later() {
	m.mu.Lock()
	m.st.Hidden = true
	m.mu.Unlock()
}

func (m *updateManager) SetMode(mode update.Mode) {
	m.mu.Lock()
	m.pol.Mode = mode
	m.save()
	m.mu.Unlock()
}

func (m *updateManager) SetChannel(c update.Channel) {
	m.mu.Lock()
	changed := m.pol.Channel != c
	m.pol.Channel = c
	m.save()
	m.mu.Unlock()
	if changed {
		m.Check()
	}
}

func (m *updateManager) CanRestore() bool {
	return m.inErr == nil && m.in.HasPrevious()
}

func (m *updateManager) Restore() (bool, error) {
	if m.inErr != nil {
		return false, updates.ErrNotInstalled
	}
	a, err := m.in.Restore()
	if err != nil {
		return false, err
	}
	if a.Exit != nil {
		atExit = a.Exit
		return false, nil
	}
	os.Setenv(EnvEditorExe, m.in.Launch)
	return true, nil
}

func (m *updateManager) Page() string {
	return DownloadPage
}

var _ updateui.Host = (*updateManager)(nil)

// updates is the update service of an editor window; a headless editor checks nothing.
func (s *Session) updates(cfg Config) updateui.Host {
	if cfg.Headless {
		return nil
	}
	return updatesFor(prefsDir(cfg))
}
