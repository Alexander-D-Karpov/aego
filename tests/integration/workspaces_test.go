package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/play"
	"aego/editor/core/session"
	"aego/editor/core/workspace"
	"aego/editor/shell"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/project"
	"aego/ui/core"
)

type fullscreener interface {
	Fullscreen() bool
}

func workspaceSession(t *testing.T) *edit.Session {
	t.Helper()
	p, err := project.Load(copyExample(t, "platformer"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()}, combatModule{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Step()
	return s
}

func TestWorkspaceSwitchKeepsDocuments(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	ws := s.App.Workspace()
	d := ctx.Doc()
	if d == nil {
		t.Fatal("platformer must open its main scene")
	}
	d.MarkDirty()
	docs := len(ctx.Docs.Docs())
	for _, p := range shell.BuiltinWorkspaces() {
		if !ws.Switch(p.ID) {
			t.Fatalf("cannot switch to %s", p.ID)
		}
		s.Step()
		if ctx.Doc() != d || len(ctx.Docs.Docs()) != docs || !d.Dirty() {
			t.Fatalf("switching to %s changed the documents", p.ID)
		}
	}
	ws.Switch(workspace.Programming)
	if ws.Tree.Has(workspace.Hierarchy) || !ws.Tree.Has(workspace.Scene) {
		t.Fatal("programming hides the hierarchy and keeps the scene")
	}
	ws.Switch(workspace.Art)
	if ws.Tree.Has(workspace.Console) || !ws.Tree.Has(workspace.Assets) {
		t.Fatal("art hides the console and keeps the assets")
	}
}

func TestWorkspaceKeysWorkInTextFields(t *testing.T) {
	s := workspaceSession(t)
	ws := s.App.Workspace()
	plat, ok := s.Host.Engine.Platform().(*headless.Platform)
	if !ok {
		t.Skip("needs the headless platform")
	}
	s.App.Context().UI.SetFocus(core.IDOf("test.field"))
	plat.Push(platform.Event{Kind: platform.EvKeyDown, Key: input.KeyEvent{Scan: input.Key2, Mods: input.ModCtrl}})
	s.Step()
	if ws.Current().ID != workspace.Programming {
		t.Fatalf("Ctrl+2 with a focused field switched to %s", ws.Current().ID)
	}
}

func TestFullScreenPlay(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	pc := s.App.Play()
	win, ok := s.Host.Engine.Window().(fullscreener)
	if !ok {
		t.Skip("window cannot report full screen")
	}
	if err := ctx.Commands.Run(shell.CommandFullscreenPlay); err != nil {
		t.Fatal(err)
	}
	s.Step()
	if !s.App.FullScreen() || !win.Fullscreen() || !pc.Captured() || pc.State() == play.Stopped {
		t.Fatal("F11 must start Play in full screen with input captured")
	}
	pc.SetCapture(false, mathx.Vec2{}, mathx.Vec2{})
	s.Step()
	if s.App.FullScreen() || win.Fullscreen() {
		t.Fatal("releasing the input must leave full screen")
	}
	if pc.State() == play.Stopped {
		t.Fatal("leaving full screen must keep Play running")
	}
	if err := ctx.Commands.Run(shell.CommandFullscreenPlay); err != nil {
		t.Fatal(err)
	}
	s.Step()
	if !win.Fullscreen() {
		t.Fatal("F11 during Play must enter full screen")
	}
	if err := ctx.Commands.Run(command.PlayStop); err != nil {
		t.Fatal(err)
	}
	s.Step()
	if s.App.FullScreen() || win.Fullscreen() || pc.State() != play.Stopped {
		t.Fatal("Stop must leave full screen")
	}
}

func TestHandoffKeepsWorkspace(t *testing.T) {
	s := workspaceSession(t)
	s.App.Workspace().Switch(workspace.Art)
	if err := s.App.Handoff(); err != nil {
		t.Fatal(err)
	}
	h, ok := session.Take(s.Host.Project.Root)
	if !ok || h.State.Workspace != workspace.Art {
		t.Fatal("handoff must carry the current workspace")
	}
}
