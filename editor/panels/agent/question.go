package agent

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

// questionBody lays out an ask_user card: the question, one row per option and the user's own answer; with draw false it only measures (11b5a).
func (p *Panel) questionBody(ui *core.Context, b *block, r mathx.Rect, draw bool) float32 {
	th := &ui.Theme
	row := th.RowHeight
	line := ui.LineHeight()
	pad := th.Padding
	q := p.question(b.question)
	if q == nil {
		return row
	}
	w := r.W - 2*pad
	if !draw || b.lines == nil {
		b.lines = widgets.WrapLines(ui, q.Text, w)
	}
	pending := q.State == agentui.Pending
	if draw {
		ui.RoundRect(r, th.Radius.Button, th.Agent.Tool)
		col := th.Border
		if pending {
			col = th.Agent.Presence
		}
		ui.List().RectOutline(r, col, 1)
	}
	y := r.Y + pad
	if draw {
		hr := mathx.Rect{X: r.X + pad, Y: y, W: w, H: row}
		widgets.IconAt(ui, mathx.Rect{X: hr.X, Y: hr.Y + (row-14)/2, W: 14, H: 14}, "agent.avatar", th.Agent.Presence)
		ui.Text(i18n.Tr("editor.agent.asks", "The agent asks"), mathx.Vec2{X: hr.X + 20, Y: ui.TextY(hr)}, th.TextDim)
	}
	y += row
	for _, l := range b.lines {
		if draw {
			ui.Text(l, mathx.Vec2{X: r.X + pad, Y: y}, th.Text)
		}
		y += line
	}
	y += th.Spacing
	if !pending {
		if draw {
			ar := mathx.Rect{X: r.X + pad, Y: y, W: w, H: row}
			icon, col, text := "status.ok", th.Ok, q.Answer
			if q.State != agentui.Applied {
				icon, col, text = "status.warning", th.TextDisabled, i18n.Tr("editor.agent.not_answered", "Not answered")
			}
			widgets.IconAt(ui, mathx.Rect{X: ar.X, Y: ar.Y + (row-14)/2, W: 14, H: 14}, icon, col)
			text = strings.TrimPrefix(strings.TrimPrefix(text, "The user chose: "), "The user answered: ")
			ui.Text(widgets.Elide(ui, text, ar.W-20), mathx.Vec2{X: ar.X + 20, Y: ui.TextY(ar)}, th.TextDim)
		}
		return y + row + pad - r.Y
	}
	digit := 0
	if draw && ui.Focus() == 0 {
		digit = keyDigit(ui)
	}
	for i := range q.Options {
		o := &q.Options[i]
		h := row + 4
		if o.Detail != "" {
			h += line
		}
		if draw {
			or := mathx.Rect{X: r.X + pad, Y: y, W: w, H: h}
			id := ui.IDInt(3000 + i)
			hit := ui.Clicked(id, or) || digit == i+1
			on := i < len(b.pick) && b.pick[i]
			fill := th.Widget
			if ui.Hot() == id {
				fill = th.WidgetHot
			}
			if on {
				fill = th.SelectionDim
			}
			ui.RoundRect(or, th.Radius.Button, fill)
			badge := mathx.Rect{X: or.X + 6, Y: or.Y + (row+4-18)/2, W: 18, H: 18}
			if q.Multiple {
				ui.Box(badge, 3, th.Background, th.Border)
				if on {
					widgets.IconAt(ui, badge, "ui.check", th.Accent)
				}
			} else {
				ui.RoundRect(badge, 4, th.Background)
				ui.TextIn(badge, itoa(i+1), th.TextDim, core.AlignCenter)
			}
			tx := badge.X + badge.W + 8
			ui.Text(widgets.Elide(ui, o.Label, or.X+or.W-tx-6), mathx.Vec2{X: tx, Y: ui.TextY(mathx.Rect{Y: or.Y + 2, H: row})}, th.Text)
			if o.Detail != "" {
				ui.Text(widgets.Elide(ui, o.Detail, or.X+or.W-tx-6), mathx.Vec2{X: tx, Y: or.Y + row + 1}, th.TextDim)
			}
			if hit {
				if q.Multiple {
					b.pick[i] = !on
				} else {
					p.d.Host.Answer(q.ID, []int{i}, "")
				}
			}
		}
		y += h + 4
	}
	if q.Other || q.Multiple {
		if draw {
			label := i18n.Tr("editor.agent.answer", "Answer")
			bw := ui.TextSize(label).X + 24
			fr := mathx.Rect{X: r.X + pad, Y: y, W: w - bw - 6, H: row}
			id := ui.ID("other")
			if q.Other {
				widgets.TextFieldRect(ui, id, fr, &b.other, true)
				if b.other == "" && ui.Focus() != id {
					ui.Text(widgets.Elide(ui, i18n.Tr("editor.agent.other_hint", "Something else…"), fr.W-8), mathx.Vec2{X: fr.X + 4, Y: ui.TextY(fr)}, th.TextDisabled)
				}
			}
			var picks []int
			for i, on := range b.pick {
				if on {
					picks = append(picks, i)
				}
			}
			ready := len(picks) > 0 || strings.TrimSpace(b.other) != ""
			if primary(ui, ui.ID("answer"), mathx.Rect{X: r.X + pad + w - bw, Y: y, W: bw, H: row}, label, ready) {
				p.d.Host.Answer(q.ID, picks, strings.TrimSpace(b.other))
			}
		}
		y += row + th.Spacing
	}
	return y + pad - r.Y
}
