//go:build aego_golden

package golden

import (
	"os"
	"runtime"
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/workspace"
	animpanel "aego/editor/panels/animation"
	"aego/project"
	"aego/sdk"
	"aego/tests/fixture"
)

func platformerSession(t *testing.T) *edit.Session {
	t.Helper()
	runtime.LockOSThread()
	p, err := project.Load(fixture.Copy(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Hidden: true, NoWatch: true, Width: 1000, Height: 620, Logger: log.Nop()}, []sdk.Module{fixture.Platformer{}}...)
	if err != nil {
		t.Skipf("no GL context available: %v", err)
	}
	t.Cleanup(s.Close)
	if name := s.Engine.Caps().Renderer; !softwareRenderer(name) && os.Getenv("AEGO_GOLDEN_ALLOW_HARDWARE") == "" {
		t.Fatalf("goldens require a software rasterizer, got %q", name)
	}
	s.Step()
	return s
}

// The inspector of a sliced sheet: Import settings, the sheet with its cells and the slicing setup.
func TestEditorSlicerGolden(t *testing.T) {
	s := platformerSession(t)
	id, ok := s.Pipeline.Lookup("assets/sprites/slash.png")
	if !ok {
		t.Fatal("no assets/sprites/slash.png")
	}
	s.App.Workspace().Show(workspace.Inspector)
	s.App.Data().Inspect(id)
	s.App.Workspace().ToggleMaximize(workspace.Inspector)
	for range 30 {
		s.Step()
	}
	r, ok := s.App.Workspace().Tree.PanelRect(workspace.Inspector)
	if !ok {
		t.Fatal("the inspector is not shown")
	}
	rectShot(t, s, r, "editor_slicer")
}

// The dope sheet of the run clip shows the builtin runner frames as thumbnails on its sprite keys.
func TestEditorRunnerThumbnailsGolden(t *testing.T) {
	s := platformerSession(t)
	ctx := s.App.Context()
	sc := ctx.Scene()
	ctx.Select(sc.ID(sc.Find(sc.Root(), "Player")))
	s.App.Workspace().Show(animpanel.ID)
	for range 5 {
		s.Step()
	}
	if !s.App.AnimationPanel().PickClip("assets/anim/player_run.aeanim") {
		t.Fatal("the Player has no player_run clip")
	}
	s.App.Workspace().ToggleMaximize(animpanel.ID)
	for range 60 {
		s.Step()
	}
	r, ok := s.App.Workspace().Tree.PanelRect(animpanel.ID)
	if !ok {
		t.Fatal("the Animation panel is not shown")
	}
	rectShot(t, s, r, "editor_runner_thumbs")
}
