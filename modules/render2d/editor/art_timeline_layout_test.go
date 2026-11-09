package editor

import (
	stdctx "context"
	"fmt"
	"testing"

	"aego/art"
	"aego/core/mathx"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/theme"
)

// These exercise the actual immediate-mode controls with synthetic input,
// not an OS/GPU or screenshot acceptance test.
func TestArtTimelineResponsiveControlsAndCelGrid(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, width := range []float32{160, 240, 640} {
			t.Run(fmt.Sprintf("light=%v/width=%.0f", light, width), func(t *testing.T) {
				panel, state, _, area := artDurationFixture(t)
				ui, model, session := panel.ctx.UI, panel.model, panel.model.Session
				if light {
					ui.Theme = theme.Light()
				} else {
					ui.Theme = theme.Editor()
				}
				font := text.NewFont(14, 1, 16, 12)
				for r := rune(32); r <= 0x04ff; r++ {
					font.SetGlyph(r, text.FontGlyph{Advance: 8})
				}
				ui.SetFont(font)
				area.W, area.H = width, 240
				first := session.Frame
				if _, err := model.Execute(stdctx.Background(), art.Operation{Kind: "add-frame", DurationMS: 100}); err != nil {
					t.Fatal(err)
				}
				frames := session.Document.Info().Frames
				if err := session.Select("", first, ""); err != nil {
					t.Fatal(err)
				}
				var overflow []string
				ui.Overflow = func(label string, _, _ float32) { overflow = append(overflow, label) }
				draw := func() {
					ui.Begin(state, mathx.Vec2{X: width, Y: area.H}, 1)
					panel.timeline(ui, area, true)
					ui.End()
				}
				click := func(point mathx.Vec2) {
					t.Helper()
					state.BeginFrame()
					state.ApplyMouseMove(point, mathx.Vec2{})
					state.ApplyMouseDown(point, input.MouseLeft)
					draw()
					state.BeginFrame()
					state.ApplyMouseUp(point, input.MouseLeft)
					draw()
				}
				state.BeginFrame()
				draw()
				pad, row, gap := ui.Theme.Padding, ui.Theme.RowHeight, ui.Theme.Spacing
				contentW := width - 2*pad
				toolbar := mathx.Rect{X: pad, Y: pad, W: min(contentW, 400), H: row}
				before := model.History.Cursor()
				click(artCell(toolbar, 0, 4).Center())
				if !panel.view.playing {
					t.Fatal("Play did not activate")
				}
				click(artCell(toolbar, 0, 4).Center())
				click(artCell(toolbar, 1, 4).Center())
				if panel.view.playing || !panel.view.onion || model.History.Cursor() != before {
					t.Fatal("Pause or Onion changed document history")
				}
				click(artCell(toolbar, 3, 4).Center())
				if session.Frame != frames[1].ID || model.History.Cursor() != before {
					t.Fatal("Next did not select the second frame without editing")
				}
				click(artCell(toolbar, 2, 4).Center())
				if session.Frame != first {
					t.Fatal("Previous did not return to the first frame")
				}
				controls := mathx.Rect{X: pad, Y: pad + 2*(row+gap), W: min(contentW, 480), H: row}
				click(artCell(controls, 0, 4).Center())
				if len(session.Document.Info().Frames) != 3 || model.History.Cursor() != before+1 {
					t.Fatal("Add Frame did not create exactly one history entry")
				}
				if err := model.SessionUndo(); err != nil {
					t.Fatal(err)
				}
				if err := session.Select("", first, ""); err != nil {
					t.Fatal(err)
				}
				state.BeginFrame()
				draw()
				labelW := min(float32(150), max(float32(86), contentW*.24))
				labelW = min(labelW, max(float32(40), contentW-ui.Theme.ScrollbarW-48))
				columns := max(1, panel.view.timelineColumns)
				cellW := (contentW - labelW - ui.Theme.ScrollbarW) / float32(columns)
				gridY := pad + 4*(row+gap)
				click(mathx.Vec2{X: pad + labelW + (cellW-2)/2, Y: gridY + row + gap + row/2})
				if len(panel.view.timelineCells) != 1 || panel.view.timelineCells[0].Frame != first || model.History.Cursor() != before || panel.err != nil {
					t.Fatal("cel grid did not select the canonical frame without editing", panel.err)
				}
				if len(overflow) != 0 {
					t.Fatalf("timeline labels overflow their bounds: %v", overflow)
				}
			})
		}
	}
}

func TestArtTimelineShortDockScrollsToCelGrid(t *testing.T) {
	panel, state, _, area := artDurationFixture(t)
	ui := panel.ctx.UI
	area.W, area.H = 240, 96
	draw := func() {
		ui.Begin(state, mathx.Vec2{X: area.W, Y: area.H}, 1)
		panel.timeline(ui, area, true)
		ui.End()
	}
	state.BeginFrame()
	draw()
	state.BeginFrame()
	state.ApplyWheel(mathx.Vec2{X: 30, Y: 30}, mathx.Vec2{Y: -2})
	draw()
	if panel.view.timelinePanelScroll <= 0 || panel.view.timelineScroll != 0 {
		t.Fatalf("short dock did not scroll its controls independently of the layer grid: panel=%v grid=%v", panel.view.timelinePanelScroll, panel.view.timelineScroll)
	}
	pad, row, gap := ui.Theme.Padding, ui.Theme.RowHeight, ui.Theme.Spacing
	contentW := area.W - 2*pad - ui.Theme.ScrollbarW - gap
	labelW := min(float32(150), max(float32(86), contentW*.24))
	labelW = min(labelW, max(float32(40), contentW-ui.Theme.ScrollbarW-48))
	cellW := (contentW - labelW - ui.Theme.ScrollbarW) / float32(max(1, panel.view.timelineColumns))
	gridY := pad + 4*(row+gap) - panel.view.timelinePanelScroll
	point := mathx.Vec2{X: pad + labelW + (cellW-2)/2, Y: gridY + row + gap + row/2}
	state.BeginFrame()
	state.ApplyMouseMove(point, mathx.Vec2{})
	state.ApplyMouseDown(point, input.MouseLeft)
	draw()
	state.BeginFrame()
	state.ApplyMouseUp(point, input.MouseLeft)
	draw()
	if len(panel.view.timelineCells) != 1 || panel.err != nil {
		t.Fatal("scrolling did not make the cel grid interactive", panel.err)
	}
}
