package settings

import (
	"testing"

	"aego/core/mathx"
	"aego/engine"
	es "aego/engine/settings"
	raw "aego/platform/input"
	"aego/ui/core"
)

func TestEscapeOpeningEdgeDoesNotAlsoCloseView(t *testing.T) {
	world := engine.NewWorld("game")
	view := NewView(&es.Store{}, world)
	ui := core.New(nil)
	var in raw.State
	draw := func() {
		// A small surface exercises the menu keyboard dispatch without a text atlas.
		size := mathx.Vec2{X: 128, Y: 96}
		ui.Begin(&in, size, 1)
		view.DrawUI(ui, size)
		ui.End()
	}
	nextFrame := func() {
		world.Time.Begin(1.0 / 60)
		in.BeginFrame()
	}

	nextFrame()
	in.ApplyKeyDown(raw.KeyEscape, 0, 0, false)
	view.Open() // Gameplay Update bound to Escape / Start.
	draw()      // RenderUI receives the same input snapshot.
	if !view.Opened() || !world.Time.Paused() {
		t.Fatal("opening Escape was handled twice")
	}
	nextFrame()
	in.ApplyKeyDown(raw.KeyEscape, 0, 0, true)
	draw()
	if !view.Opened() {
		t.Fatal("keyboard repeat closed the settings menu")
	}
	in.ApplyKeyUp(raw.KeyEscape, 0, 0)
	nextFrame()
	in.ApplyKeyDown(raw.KeyEscape, 0, 0, false)
	in.ApplyKeyUp(raw.KeyEscape, 0, 0)
	draw()
	if view.Opened() || world.Time.Paused() {
		t.Fatal("a fresh Escape tap while paused must close the menu and resume gameplay")
	}
}
