package widgets

import (
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

func TestToggleMenuItemStaysOpen(t *testing.T) {
	ui, in := core.New(nil), &input.State{}
	id := core.IDOf("toggle-menu")
	var toggle, normal mathx.Rect
	on := false
	frame := func(at mathx.Vec2, down, up bool) {
		in.BeginFrame()
		in.ApplyMouseMove(at, mathx.Vec2{})
		if down {
			in.ApplyMouseDown(at, input.MouseLeft)
		}
		if up {
			in.ApplyMouseUp(at, input.MouseLeft)
		}
		ui.Begin(in, mathx.V2(800, 600), 1)
		if ui.BeginPopup(id, 200) {
			cur := *ui.Cursor()
			toggle, normal = cur.Next(0), cur.Next(0)
			if Entry(ui, Item{Label: "Nightly Updates", Enabled: true, Checked: on, KeepOpen: true}) {
				on = !on
			}
			Entry(ui, Item{Label: "Ordinary action", Enabled: true})
			ui.EndPopup()
		}
		ui.End()
	}
	ui.OpenPopup(id, mathx.V2(20, 20))
	frame(mathx.V2(0, 0), false, false)
	frame(mathx.V2(0, 0), false, false)
	center := func(r mathx.Rect) mathx.Vec2 { return mathx.V2(r.X+r.W/2, r.Y+r.H/2) }
	for _, want := range []bool{true, false} {
		at := center(toggle)
		frame(at, true, false)
		frame(at, false, true)
		if on != want || !ui.PopupOpen(id) {
			t.Fatalf("toggle = %v, popup open = %v; want %v and open", on, ui.PopupOpen(id), want)
		}
	}
	at := center(normal)
	frame(at, true, false)
	frame(at, false, true)
	if ui.PopupOpen(id) {
		t.Fatal("ordinary action must still close the menu")
	}
}
