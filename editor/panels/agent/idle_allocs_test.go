package agent

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

// Measure the panel itself independently of asynchronous editor work.
func TestIdlePanelDrawDoesNotAllocate(t *testing.T) {
	p := providerPanel(t, newProviderTestHost())
	ui := core.New(text.NewFont(14, 1, 18, 14))
	ui.Theme = theme.Editor()
	in := &input.State{}
	r := mathx.Rect{W: 420, H: 520}
	frame := func(draw bool) {
		in.BeginFrame()
		ui.Begin(in, mathx.V2(800, 600), 1)
		ui.Region(r)
		if draw {
			p.Draw(&services.Frame{UI: ui}, r)
		}
		ui.PopCursor()
		ui.End()
	}
	for range 60 {
		frame(false)
		frame(true)
	}
	base := testing.AllocsPerRun(100, func() { frame(false) })
	with := testing.AllocsPerRun(100, func() { frame(true) })
	if with > base {
		t.Fatalf("idle panel adds allocations: base=%v with panel=%v", base, with)
	}
}
