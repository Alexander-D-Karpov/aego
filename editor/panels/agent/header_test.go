package agent

import (
	"strconv"
	"testing"

	"aego/core/mathx"
	"aego/editor/core/follow"
	"aego/editor/core/presence"
	edselection "aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/platform/input"
	"aego/render/text"
	"aego/ui/core"
	"aego/ui/theme"
)

type toolbarTestHost struct {
	*providerTestHost
	auto bool
}

func (h *toolbarTestHost) AutoApply() bool     { return h.auto }
func (h *toolbarTestHost) SetAutoApply(v bool) { h.auto = v }

func TestAgentHeaderControlsRemainUsableWhenNarrow(t *testing.T) {
	for _, width := range []float32{420, 260} {
		t.Run(strconv.Itoa(int(width)), func(t *testing.T) {
			h := &toolbarTestHost{providerTestHost: newProviderTestHost()}
			p := providerPanel(t, h.providerTestHost)
			p.d.Host = h
			p.d.Follow = follow.New(presence.New(), edselection.New(), nil)
			ui := core.New(text.NewFont(14, 1, 18, 14))
			ui.Theme = theme.Editor()
			in := &input.State{}
			r := mathx.Rect{W: width, H: 520}
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
				ui.Region(r)
				p.Draw(&services.Frame{UI: ui}, r)
				ui.PopCursor()
				ui.End()
			}
			click := func(at mathx.Vec2) { frame(at, true, false); frame(at, false, true); frame(at, false, false) }
			frame(mathx.V2(0, 0), false, false)
			pad, row := ui.Theme.Padding, ui.Theme.RowHeight
			y := row + pad + pad + row/2
			more := mathx.V2(width-pad-row/2, y)
			followAt := mathx.V2(width-pad-row-4-row/2, y)
			right := width - pad - 2*(row+4)
			auto := mathx.V2(right-row/2, y)
			click(auto)
			if !h.auto {
				t.Fatal("auto-apply control is missing or overlaps its neighbours")
			}
			click(followAt)
			if !p.d.Follow.On() {
				t.Fatal("follow button is missing or overlaps its neighbours")
			}
			p.d.Follow.UserActive(1)
			click(followAt)
			if !p.d.Follow.Active() {
				t.Fatal("the follow button must resume a paused follow")
			}
			click(more)
			if !ui.PopupOpen(p.menu) {
				t.Fatal("session menu button is missing or overlaps its neighbours")
			}
			ui.CloseAllPopups()
			settings := mathx.V2(p.settingsRect.X+p.settingsRect.W/2, p.settingsRect.Y+p.settingsRect.H/2)
			click(settings)
			if !ui.PopupOpen(p.providerSettings) {
				t.Fatal("settings button is missing or overlaps the provider picker")
			}
		})
	}
}
