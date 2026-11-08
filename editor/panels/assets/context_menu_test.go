package assets

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/dnd"
)

func TestGridContextMenuOnEmptyCellsAndFolder(t *testing.T) {
	for _, tc := range []struct {
		name string
		at   mathx.Vec2
		want string
	}{
		{"empty cell beside folder", mathx.Vec2{X: 150, Y: 40}, "assets"},
		{"gap above folder", mathx.Vec2{X: 40, Y: 1}, "assets"},
		{"folder", mathx.Vec2{X: 40, Y: 40}, "assets/one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui, in := core.New(nil), &input.State{}
			p := New(Deps{})
			p.dirs = []string{"assets", "assets/one"}
			p.f = &services.Frame{UI: ui, DnD: &dnd.State{}}
			p.dirMenuID = core.IDOf("test-assets-directory-menu")
			draw := func() {
				ui.Begin(in, mathx.Vec2{X: 300, Y: 300}, 1)
				p.drawGrid(mathx.Rect{W: 210, H: 250})
				ui.End()
			}
			in.BeginFrame()
			draw()
			in.BeginFrame()
			in.ApplyMouseMove(tc.at, mathx.Vec2{})
			in.ApplyMouseDown(tc.at, input.MouseRight)
			draw()
			if !ui.PopupOpen(p.dirMenuID) || p.menuDir != tc.want {
				t.Fatalf("right click at %v: open=%v directory=%q, want %q", tc.at, ui.PopupOpen(p.dirMenuID), p.menuDir, tc.want)
			}
		})
	}
}
