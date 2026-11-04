package editor

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/ui/core"
)

func TestImageChannelPickerFocusesSearchAndAssignsWithEnter(t *testing.T) {
	v := newImageState()
	ui, in := core.New(nil), &input.State{}
	field := mathx.Rect{X: 10, Y: 10, W: 240, H: 24}
	point := mathx.Vec2{X: 30, Y: 18}
	frame := ids.AssetID{Lo: 42}
	normal := ids.AssetID{Lo: 99}
	labels := []string{"Inherit / discover", "Disabled explicitly", "normal walking frame"}
	refs := []ids.AssetID{{}, {}, normal}
	draw := func() (ids.AssetID, bool, bool) {
		ui.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
		r, inherit, ok := v.chooseChannel(ui, field, "Normal", labels, refs, 0, frame)
		ui.End()
		return r, inherit, ok
	}
	in.BeginFrame()
	in.ApplyMouseMove(point, mathx.Vec2{})
	in.ApplyMouseDown(point, input.MouseLeft)
	draw()
	in.BeginFrame()
	in.ApplyMouseUp(point, input.MouseLeft)
	draw()
	if ui.Focus() == 0 {
		t.Fatal("asset picker did not focus its search field")
	}
	in.BeginFrame()
	in.ApplyText([]byte("walking"))
	draw()
	in.BeginFrame()
	in.ApplyKeyDown(input.KeyEnter, 0, 0, false)
	ref, inherit, picked := draw()
	if !picked || inherit || ref != normal {
		t.Fatalf("typed search and Enter did not assign the stable frame ID: %s %v %v", ref, inherit, picked)
	}
}
