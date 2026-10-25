package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/mathx"
	"aego/editor/shell/addpalette"
	"aego/platform"
	"aego/platform/input"
)

func click(t *testing.T, s *edit.Session, at mathx.Vec2) {
	t.Helper()
	push(t, s,
		platform.Event{Kind: platform.EvMouseMove, Mouse: input.MouseEvent{Pos: at}},
		platform.Event{Kind: platform.EvMouseDown, Mouse: input.MouseEvent{Pos: at, Button: input.MouseLeft}},
		platform.Event{Kind: platform.EvMouseUp, Mouse: input.MouseEvent{Pos: at, Button: input.MouseLeft}},
	)
}

// The Create entry of the menu bar opens the Create palette with one click, and it stays open.
func TestMenuBarCreateOpensPalette(t *testing.T) {
	s := workspaceSession(t)
	pal := s.App.AddPalette()
	for x := float32(4); x < 400; x += 8 {
		click(t, s, mathx.Vec2{X: x, Y: 8})
		s.Step()
		if pal.Active() {
			if pal.Mode() != addpalette.Create {
				t.Fatalf("x %v opened the palette in mode %v", x, pal.Mode())
			}
			return
		}
		push(t, s, key(input.KeyEscape, 0)...)
	}
	t.Fatal("no click on the menu bar opened the Create palette")
}
