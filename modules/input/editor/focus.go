package editor

import (
	"strconv"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/engine"
	"aego/engine/i18n"
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
	return i18n.Tr("i18n.input.focus.title", "Input Focus")
}

func (p *Focus) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	var s *input.Service
	if p.world != nil {
		s = input.Of(p.world())
	}
	if s == nil {
		widgets.LabelDim(ui, i18n.Tr("i18n.input.the_context_stack_of_the", "The context stack of the running game appears here while Play runs"))
		return
	}
	widgets.Label(ui, i18n.Tr("i18n.input.context_stack_top_first", "Context stack, top first"))
	for k, x := range s.Stack() {
		line := i18n.Tr("i18n.input.v_name_priority_priority", "{v}. {name}   priority {priority}", i18n.Str("v", strconv.Itoa(k+1)), i18n.Str("name", x.Name), i18n.Str("priority", strconv.Itoa(x.Priority)))
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
