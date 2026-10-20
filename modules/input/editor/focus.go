package editor

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/engine"
	"aego/modules/input"
	"aego/ui/widgets"
)

type Focus struct {
	world  func() *engine.World
	player int
}

func NewFocus(world func() *engine.World) *Focus {
	return &Focus{world: world}
}

func (*Focus) ID() string {
	return FocusID
}

func (*Focus) Title() string {
	return "Input Focus"
}

func (p *Focus) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	var s *input.Service
	if p.world != nil {
		s = input.Of(p.world())
	}
	if s == nil {
		widgets.LabelDim(ui, "The context stack of the running game appears here while Play runs")
		return
	}
	widgets.Label(ui, "Context stack, top first")
	for k, x := range s.Stack() {
		line := strconv.Itoa(k+1) + ". " + x.Name + "   priority " + strconv.Itoa(x.Priority)
		if x.Consume {
			line += "   consumes its controls"
		}
		widgets.LabelDim(ui, line)
	}
	widgets.Separator(ui)
	cols := [4]float32{0, 0.25, 0.45, 0.6}
	row := func(vals [4]string, col mathx.Color) {
		rr := ui.Cursor().Next(0)
		ui.PushClip(rr)
		for i, v := range vals {
			ui.Text(v, mathx.Vec2{X: rr.X + rr.W*cols[i] + 4, Y: ui.TextY(rr)}, col)
		}
		ui.PopClip()
	}
	row([4]string{"action", "context", "state", "blocked by"}, th.TextDim)
	s.Each(p.player, func(i input.Info) {
		state := "inactive"
		switch {
		case i.Active && i.Down:
			state = "held"
		case i.Active:
			state = "idle"
		}
		blocked, col := "", th.Text
		if i.Blocked != "" {
			blocked = i.Blocked + " consumes " + i.Why
			col = th.Warn
		}
		if !i.Active {
			col = th.TextDisabled
		}
		row([4]string{i.Name, i.Context, state, blocked}, col)
	})
}
