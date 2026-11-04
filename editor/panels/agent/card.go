package agent

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

// applyStrip reports whether the block is the first of several waiting cards, which gets "Apply all" above it.
func (p *Panel) applyStrip(b *block) bool {
	if b.card <= 0 || p.applicable() < 2 {
		return false
	}
	c := p.card(b.card)
	if c == nil || c.State != agentui.Pending {
		return false
	}
	for i := range p.blocks {
		o := &p.blocks[i]
		if o.kind != blockCard || o.card <= 0 {
			continue
		}
		if oc := p.card(o.card); oc != nil && oc.State == agentui.Pending {
			return o == b
		}
	}
	return false
}

// pastLines are the change lines saved for a card of a past session.
func pastLines(b *block) []string {
	if b.text == "" {
		return nil
	}
	lines := strings.Split(b.text, "\n")
	return lines[1:]
}

// cardBody lays out an open card; with at nil it only measures.
func (p *Panel) cardBody(ui *core.Context, b *block, w float32, at *mathx.Rect) float32 {
	th := &ui.Theme
	row := th.RowHeight
	h := th.Spacing
	c := p.card(b.card)
	if c == nil {
		n := min(len(pastLines(b)), maxRows)
		if at != nil {
			y := at.Y + h
			for _, l := range pastLines(b)[:n] {
				ui.Text(widgets.Elide(ui, l, w-16), mathx.Vec2{X: at.X + 8, Y: ui.TextY(mathx.Rect{Y: y, H: row})}, th.TextDim)
				y += row
			}
		}
		return h + float32(n)*row + th.Spacing
	}
	n, hidden := diffLines(c.Rows, b.all)
	diffH := float32(n) * row
	if hidden > 0 {
		diffH += row
	}
	if at != nil {
		p.drawDiff(ui, c.Rows, mathx.Rect{X: at.X + 8, Y: at.Y + h, W: w - 16}, &b.all)
	}
	h += diffH
	var extra []string
	if b.all {
		extra = append(extra, c.Notes...)
	}
	extra = append(extra, c.Problems...)
	if c.State == agentui.Failed && c.Err != "" {
		extra = append(extra, c.Err)
	}
	for _, l := range extra {
		if at != nil {
			lr := mathx.Rect{X: at.X + 8, Y: at.Y + h, W: w - 16, H: row}
			col := th.TextDim
			if strings.HasPrefix(l, "error:") || l == c.Err {
				col = th.Error
			} else if strings.HasPrefix(l, "warning:") {
				col = th.Warn
			}
			ui.Text(widgets.Elide(ui, l, lr.W), mathx.Vec2{X: lr.X, Y: ui.TextY(lr)}, col)
		}
		h += row
	}
	if c.State == agentui.Pending || c.CanUndo || len(selection(c)) > 0 {
		if at != nil {
			p.cardButtons(ui, b, c, mathx.Rect{X: at.X + 8, Y: at.Y + h + th.Spacing, W: w - 16, H: row})
		}
		h += row + th.Spacing
	}
	return h + th.Spacing
}

// selection is what a card's Select button selects: the nodes it changed or created.
func selection(c *agentui.Card) []agentui.Change {
	if c.State != agentui.Applied || c.File == "" {
		return nil
	}
	for _, ch := range c.Applied {
		if !ch.NodeID.IsZero() {
			return c.Applied
		}
	}
	return nil
}

func (p *Panel) cardButtons(ui *core.Context, b *block, c *agentui.Card, r mathx.Rect) {
	x := r.X
	button := func(key, label string) bool {
		w := ui.TextSize(label).X + 20
		hit := widgets.ButtonRect(ui, ui.ID(key), mathx.Rect{X: x, Y: r.Y, W: w, H: r.H}, label)
		x += w + 6
		return hit
	}
	if c.State == agentui.Pending {
		label := i18n.Tr("editor.agent.apply", "Apply")
		w := ui.TextSize(label).X + 24
		if primary(ui, ui.ID("apply"), mathx.Rect{X: x, Y: r.Y, W: w, H: r.H}, label, true) {
			p.d.Host.Decide(c.ID, true)
		}
		x += w + 6
		if button("dry", i18n.Tr("editor.agent.dry_run", "Dry-run")) {
			b.all = !b.all
			b.dirty()
		}
		widgets.Tooltip(ui, ui.ID("dry"), i18n.Tr("editor.agent.dry_run_tip", "Show everything the dry run reported: every change, note and problem"))
		if button("skip", i18n.Tr("editor.agent.skip", "Skip")) {
			p.d.Host.Decide(c.ID, false)
		}
		return
	}
	if c.CanUndo && button("undo", i18n.Tr("editor.agent.undo", "Undo")) {
		if err := p.d.Host.Undo(c.ID); err != nil {
			p.d.Notices.Notify(context.LevelWarn, err.Error())
		}
	}
	if sel := selection(c); len(sel) > 0 && button("select", i18n.Tr("editor.agent.select", "Select")) {
		for _, ch := range sel {
			if !ch.NodeID.IsZero() {
				p.openLink("node:" + c.File + "#" + ch.NodeID.String())
			}
		}
	}
}

// cardState is the right edge of a card's header: what happened to it.
func cardState(ui *core.Context, st agentui.State) (icon, label string, col mathx.Color) {
	th := &ui.Theme
	switch st {
	case agentui.Pending:
		return "", i18n.Tr("editor.agent.state_waiting", "waiting for you"), th.Accent
	case agentui.Approved:
		return "status.busy", i18n.Tr("editor.agent.state_applying", "applying…"), th.TextDim
	case agentui.Applied:
		return "status.ok", i18n.Tr("editor.agent.state_applied", "applied"), th.Ok
	case agentui.Failed:
		return "status.error", i18n.Tr("editor.agent.state_failed", "failed"), th.Error
	case agentui.Rejected:
		return "", i18n.Tr("editor.agent.state_skipped", "skipped"), th.TextDisabled
	case agentui.Undone:
		return "", i18n.Tr("editor.agent.state_undone", "undone"), th.TextDisabled
	case agentui.Expired:
		return "", i18n.Tr("editor.agent.state_expired", "expired"), th.TextDisabled
	}
	return "", "", th.TextDisabled
}

func (p *Panel) drawCard(ui *core.Context, b *block, r mathx.Rect) {
	th := &ui.Theme
	row := th.RowHeight
	y := r.Y
	if p.applyStrip(b) {
		sr := mathx.Rect{X: r.X, Y: y, W: r.W, H: row}
		ui.RoundRect(sr, th.Radius.Button, th.AccentDim)
		label := i18n.Tr("editor.agent.apply_all", "Apply All")
		bw := ui.TextSize(label).X + 24
		ui.Text(i18n.TrN("editor.agent.waiting_n", int64(p.applicable()), "{n} change waits for you", "{n} changes wait for you"), mathx.Vec2{X: sr.X + 8, Y: ui.TextY(sr)}, th.Text)
		if primary(ui, ui.ID("applyall"), mathx.Rect{X: sr.X + sr.W - bw, Y: sr.Y, W: bw, H: row}, label, true) {
			p.ApplyAll()
		}
		widgets.Tooltip(ui, ui.ID("applyall"), i18n.Tr("editor.agent.apply_this_answer_s_changes", "Apply this answer's changes, the waiting ones and those still to come; removals, moves and deletes still wait for you"))
		y += row + th.Spacing
	}
	box := mathx.Rect{X: r.X, Y: y, W: r.W, H: r.Y + r.H - y}
	c := p.card(b.card)
	st := agentui.Applied
	destructive := false
	if c != nil {
		st, destructive = c.State, c.Destructive
	}
	ui.RoundRect(box, th.Radius.Button, th.Agent.Tool)
	hr := mathx.Rect{X: box.X, Y: box.Y, W: box.W, H: row}
	id := ui.ID("card")
	if ui.Clicked(id, hr) {
		b.open = !b.open
		b.dirty()
	}
	if ui.Hot() == id {
		ui.RoundRect(hr, th.Radius.Button, th.WidgetHot)
	}
	widgets.Chevron(ui, mathx.Rect{X: hr.X + 2, Y: hr.Y, W: 14, H: row}, b.open, th.TextDim)
	x := hr.X + 18
	s := &b.steps[0]
	icon := s.look.Icon
	if destructive {
		icon = "status.warning"
	}
	widgets.IconAt(ui, mathx.Rect{X: x, Y: hr.Y + (row-14)/2, W: 14, H: 14}, icon, th.Text)
	x += 20
	sicon, slabel, scol := cardState(ui, st)
	sw := ui.TextSize(slabel).X + 8
	if sicon != "" {
		sw += 18
	}
	ui.Text(s.fl.get(ui, s.look.Title, max(0, hr.X+hr.W-x-sw-8)), mathx.Vec2{X: x, Y: ui.TextY(hr)}, th.Text)
	sx := hr.X + hr.W - sw
	if sicon != "" {
		widgets.IconAt(ui, mathx.Rect{X: sx, Y: hr.Y + (row-14)/2, W: 14, H: 14}, sicon, scol)
		sx += 18
	}
	ui.Text(slabel, mathx.Vec2{X: sx, Y: ui.TextY(hr)}, scol)
	if b.open {
		body := p.cardBody(ui, b, box.W, &mathx.Rect{X: box.X, Y: box.Y + row})
		y := box.Y + row + body
		for _, key := range s.look.Detail.Images {
			h := min(box.W, 160)
			p.drawImage(ui, key, mathx.Rect{X: box.X, Y: y, W: box.W, H: h})
			y += h
		}
	}
	border := th.Border
	switch {
	case st == agentui.Pending:
		border = th.Accent
	case destructive:
		border = th.Warn
	}
	ui.List().RectOutline(box, border, 1)
}
