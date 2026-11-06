//go:build aego_gltest && linux && cgo && !aego_nonative

package edit

import (
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/play"
	"aego/editor/core/workspace"
	"aego/engine"
	"aego/modules/audio"
	"aego/platform"
)

// Uses an isolated project copy and a hidden native window, not the user's editor.
func TestProfilerIdleWaitNative(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	p := platformerAuthoringProject(t)
	const width, height = 800, 600
	s, err := Open(p, Config{Width: width, Height: height, Hidden: true, NoWatch: true, Logger: log.Nop()}, platformerAuthoringModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	step := func() {
		t.Helper()
		if !s.Step() {
			t.Fatal("editor stopped")
		}
	}
	step()
	ws := s.App.Workspace()
	ws.Show(workspace.Profiler)
	ws.Focus(workspace.Profiler)
	if ws.Tree.Maximized() != workspace.Profiler && !ws.ToggleMaximize(workspace.Profiler) {
		t.Fatal("cannot maximize profiler")
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		step()
		f := s.Engine.Trace().Frame(0)
		if s.App.Idle() && f != nil && f.IdleWaitNS > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("editor never recorded an idle wait")
		}
	}
	for range 8 {
		step()
	}
	f := s.Engine.Trace().Frame(0)
	var renderEnd, waitStart uint32
	for _, ev := range f.Events {
		switch s.Engine.Trace().Name(ev.Zone) {
		case engine.SystemAppRender:
			renderEnd = ev.Start + ev.Dur
		case engine.ZoneIdleWait:
			if ev.Depth != 0 {
				t.Fatalf("idle wait nested in another zone: %+v", ev)
			}
			waitStart = ev.Start
		}
	}
	if f.IdleWaitNS <= 0 || renderEnd == 0 || waitStart < renderEnd {
		t.Fatalf("idle wait=%d render end=%d wait start=%d", f.IdleWaitNS, renderEnd, waitStart)
	}
	ui := s.App.Context().UI
	screen := mathx.Rect{W: width, H: height}
	labels := s.App.UIList().TestDrawnItems(screen, ui.Font(), "Frame ", 0, 0)
	fps := s.App.UIList().TestDrawnItems(screen, ui.Font(), "fps ", 0, 0)
	foundIdle := false
	for _, label := range labels {
		foundIdle = foundIdle || strings.Contains(label.Text, "idle wait")
	}
	if !foundIdle || len(fps) == 0 {
		t.Fatalf("idle label=%v FPS labels=%v", labels, fps)
	}
	fields := strings.Fields(fps[0].Text)
	if len(fields) < 2 {
		t.Fatalf("cannot parse FPS label %q", fps[0].Text)
	}
	shownFPS, err := strconv.ParseFloat(fields[1], 64)
	if err != nil || math.Abs(shownFPS-float64(time.Second)/float64(f.Dur)) > 3 {
		t.Fatalf("displayed FPS %q does not match frame %.2f ms", fps[0].Text, float64(f.Dur)/1e6)
	}
	t.Logf("%s; frame %.2f ms, idle wait %.2f ms, render %.2f ms", fps[0].Text, float64(f.Dur)/1e6, float64(f.IdleWaitNS)/1e6, s.Engine.Stats().RenderTime*1000)
	pc := s.App.Play()
	if err := pc.StartProject(); err != nil {
		t.Fatal(err)
	}
	s.App.Event(platform.Event{Kind: platform.EvWindowFocusLost})
	started := time.Now()
	frames := 0
	for time.Since(started) < 1200*time.Millisecond {
		step()
		frames++
		if pc.State() != play.Playing || s.App.Idle() || s.Engine.Trace().Frame(0).IdleWaitNS != 0 {
			t.Fatal("active Play entered idle after losing focus")
		}
	}
	playFPS := float64(frames) / time.Since(started).Seconds()
	fps = s.App.UIList().TestDrawnItems(screen, ui.Font(), "fps ", 0, 0)
	if len(fps) == 0 {
		t.Fatal("no FPS label during Play")
	}
	am := audio.ModuleOf(s.Engine)
	if am == nil {
		t.Fatal("audio module missing")
	}
	st := am.Stats()
	if st.Blocks == 0 || st.Underruns != 0 {
		t.Fatalf("audio pump blocks=%d underruns=%d", st.Blocks, st.Underruns)
	}
	t.Logf("Play: measured %.1f FPS over %.2f s, UI %q; pump underruns %d", playFPS, time.Since(started).Seconds(), fps[0].Text, st.Underruns)
}
