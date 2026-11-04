//go:build aego_gltest && linux && cgo && !aego_nonative

package integration

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"aego/app/agentui/backend"
	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/platform/sdl"
	"aego/project"
)

func TestExplainSceneThroughSDLPanel(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	root := copyExample(t, "platformer")
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	cmd := codexEditorProcess(t)
	s, err := edit.Open(p, edit.Config{Hidden: true, Width: 1100, Height: 820, NoWatch: true, Logger: log.Nop(), AgentBackend: backend.Codex{}}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Step()
	s.App.SetAgentSettings(agentui.ProviderSettings{Provider: "codex", Command: cmd})
	ctx := s.App.Context()
	d, err := ctx.Docs.Open(bridgeScene)
	if err != nil {
		t.Fatal(err)
	}
	sc := d.Scene()
	n := sc.Find(sc.Root(), "Player")
	sc.SetName(n, "UnsavedActor212")
	ctx.Select(sc.ID(n))
	scr, err := ctx.Docs.OpenScript("scripts/player/player.aes")
	if err != nil {
		t.Fatal(err)
	}
	scr.Code.Replace(0, 0, "// UNSAVED_SCRIPT_212\n")
	panel := s.App.Agent()
	panel.Recheck()
	ws := s.App.Workspace()
	ws.Show("agent")
	ws.Focus("agent")
	ws.ToggleMaximize("agent")
	click := func(r mathx.Rect) {
		t.Helper()
		if r.W <= 0 || r.H <= 0 {
			t.Fatal("control is not drawn")
		}
		at := center(r)
		for i := range 3 {
			if !sdl.TestPointerEvent(at.X, at.Y, i) {
				t.Fatal("cannot enqueue SDL pointer event")
			}
			s.Step()
		}
	}
	stepUntil(t, s, func() bool { return panel.ExplainRect().W > 0 && panel.Checked() })
	click(panel.ExplainRect())
	stepUntil(t, s, func() bool { return !panel.Busy() })
	names, err := agentui.List(root)
	if err != nil || len(names) != 1 {
		t.Fatalf("SDL click did not start Explain: %v %v", names, err)
	}
	entries, err := agentui.Load(root, names[0])
	if err != nil {
		t.Fatal(err)
	}
	var liveScene, liveCode bool
	for _, e := range entries {
		if e.Kind == "result" && !e.Failed && e.Tool == "scene_tree" && strings.Contains(e.Text, "UnsavedActor212") {
			liveScene = true
		}
		if e.Kind == "result" && !e.Failed && e.Tool == "script_read" && strings.Contains(e.Text, "UNSAVED_SCRIPT_212") {
			liveCode = true
		}
	}
	if !liveScene || !liveCode {
		t.Fatalf("unexpected transcript: %+v", entries)
	}
	click(panel.SettingsRect())
	if out := os.Getenv("AEGO_AGENT_PANEL_SHOT"); out != "" {
		var screen *image.RGBA
		s.Engine.CaptureFrame(func(im *image.RGBA, err error) {
			if err != nil {
				t.Fatal(err)
			}
			screen = im
		})
		s.Step()
		if screen == nil {
			t.Fatal("no captured window")
		}
		f, err := os.Create(filepath.Clean(out))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, screen); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("SDL click; current scene + dirty script read via fixture provider's real HTTP bridge; GL=%s", s.Engine.Caps().Renderer)
}
