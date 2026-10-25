package integration

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/panels/animation"
	"aego/platform"
	"aego/platform/input"
)

// A right click on the dope sheet opens the key menu, and it stays open after the button is released.
func TestAnimationKeyMenuOpensWithTheMouse(t *testing.T) {
	s := workspaceSession(t)
	ctx := s.App.Context()
	sc := ctx.Scene()
	ctx.Select(sc.ID(sc.Find(sc.Root(), "Player")))
	ws := s.App.Workspace()
	ws.Show(animation.ID)
	for range 5 {
		s.Step()
	}
	r, ok := ws.Tree.PanelRect(animation.ID)
	if !ok {
		t.Fatal("the Animation panel is not shown")
	}
	at := mathx.Vec2{X: r.X + r.W - 40, Y: r.Y + r.H - 12}
	push(t, s,
		platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at}},
		platform.Event{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: at, Button: input.MouseRight}},
		platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: at, Button: input.MouseRight}},
	)
	s.Step()
	if !s.App.AnimationPanel().KeyMenuOpen() {
		t.Fatal("the key menu did not stay open after a right click")
	}
}
