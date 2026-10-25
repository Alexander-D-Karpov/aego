package integration

import (
	"bytes"
	"testing"

	"aego/app/edit"
	"aego/core/mathx"
	"aego/editor/core/animedit"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/workspace"
	animpanel "aego/editor/panels/animation"
	"aego/platform"
	"aego/platform/input"
	"aego/scene/sceneio"
)

// dragPlayer presses on the Player with the Move tool, moves the mouse by d and releases; it returns the Player's position before and after and the new History steps.
func dragPlayer(t *testing.T, s *edit.Session, d mathx.Vec2) (before, after mathx.Vec2, added int) {
	t.Helper()
	ctx := s.App.Context()
	sc := ctx.Scene()
	pl := sc.Find(sc.Root(), "Player")
	ctx.Select(sc.ID(pl))
	s.App.Workspace().Show(workspace.Scene)
	ctx.Settings.Tool = context.ToolMove
	s.Step()
	if err := ctx.Commands.Run(command.SceneFrame); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		s.Step()
	}
	before = sc.Local(pl).Position
	steps := len(ctx.Doc().History.Steps(nil))
	at := s.App.SceneView().ScreenAt(before)
	push(t, s,
		platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at}},
		platform.Event{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: at, Button: input.MouseLeft}},
	)
	s.Step()
	push(t, s, platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at.Add(d)}})
	s.Step()
	push(t, s, platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: at.Add(d), Button: input.MouseLeft}})
	for range 3 {
		s.Step()
	}
	return before, sc.Local(pl).Position, len(ctx.Doc().History.Steps(nil)) - steps
}

// A click on the selected node with the Move tool, the hand a pixel off between press and release, leaves it where it was; a drag still moves it.
func TestClickWithMoveToolDoesNotNudge(t *testing.T) {
	before, after, added := dragPlayer(t, workspaceSession(t), mathx.Vec2{X: -1, Y: -1})
	if after != before || added != 0 {
		t.Fatalf("a click moved the Player from %v to %v with %d History steps", before, after, added)
	}
	before, after, added = dragPlayer(t, workspaceSession(t), mathx.Vec2{X: -20, Y: -20})
	if after == before || added != 1 {
		t.Fatalf("a 20 px drag left the Player at %v with %d History steps", after, added)
	}
}

// Play and Stop, Preview and Record without edits leave the saved scene byte for byte.
func TestModesLeaveTheSceneAsSaved(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	sc := ctx.Scene()
	saved := sceneio.Save(sc)
	check := func(step string) {
		t.Helper()
		for range 5 {
			s.Step()
		}
		if got := sceneio.Save(sc); !bytes.Equal(got, saved) {
			t.Fatalf("%s changed the scene", step)
		}
		if n := len(ctx.Doc().History.Steps(nil)); n != 0 {
			t.Fatalf("%s added %d History steps", step, n)
		}
	}
	if err := ctx.Commands.Run(command.PlayStart); err != nil {
		t.Fatal(err)
	}
	for range 60 {
		s.Step()
	}
	if err := ctx.Commands.Run(command.PlayStop); err != nil {
		t.Fatal(err)
	}
	sc = ctx.Scene()
	check("Play and Stop")
	ctx.Select(sc.ID(sc.Find(sc.Root(), "Player")))
	s.App.Workspace().Show(animpanel.ID)
	for range 5 {
		s.Step()
	}
	svc := ctx.Anim.For(ctx.Doc())
	for _, m := range []animedit.Mode{animedit.Preview, animedit.Record} {
		svc.SetMode(m)
		svc.SetTime(0.1)
		for range 10 {
			s.Step()
		}
		svc.SetMode(animedit.Normal)
		check(map[animedit.Mode]string{animedit.Preview: "Preview", animedit.Record: "Record"}[m])
	}
}
