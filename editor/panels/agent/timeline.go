package agent

import (
	"path"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/follow"
	"aego/editor/core/presence"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
	wpresence "aego/ui/widgets/presence"
)

// dot is one tool call on the activity timeline: where its block is and how it went.
type dot struct {
	block int
	step  int
	card  int
	col   mathx.Color
	label string
}

const dotW = 10

// dots lists the calls of the transcript in order, coloured by what they did.
func (p *Panel) dots(ui *core.Context) []dot {
	th := &ui.Theme
	p.timeline = p.timeline[:0]
	for i := range p.blocks {
		b := &p.blocks[i]
		switch b.kind {
		case blockSteps, blockFailed:
			for j := range b.steps {
				s := &b.steps[j]
				col := th.TextDim
				if s.failed {
					col = th.Error
				}
				p.timeline = append(p.timeline, dot{block: i, step: j, col: col, label: s.label})
			}
		case blockCard:
			col := th.Accent
			if c := p.card(b.card); c != nil {
				switch c.State {
				case agentui.Applied:
					col = th.Ok
				case agentui.Failed:
					col = th.Error
				case agentui.Rejected, agentui.Undone, agentui.Expired:
					col = th.TextDisabled
				}
			}
			p.timeline = append(p.timeline, dot{block: i, card: b.card, col: col, label: b.steps[0].look.Title})
		case blockQuestion:
			p.timeline = append(p.timeline, dot{block: i, col: th.Agent.Presence, label: i18n.Tr("editor.agent.asks", "The agent asks")})
		}
	}
	return p.timeline
}

// activity draws the timeline strip: a click goes to the step, a right click on a change undoes back to it.
func (p *Panel) activity(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	list := p.dots(ui)
	if len(list) == 0 {
		return
	}
	fits := max(1, int(r.W/dotW))
	first := max(0, len(list)-fits)
	x := r.X
	for i := first; i < len(list); i++ {
		d := &list[i]
		dr := mathx.Rect{X: x, Y: r.Y, W: dotW, H: r.H}
		id := ui.IDInt(4000 + i)
		hot := ui.MouseOver(id, dr)
		sz := float32(6)
		if hot {
			sz = 9
		}
		ui.RoundRect(mathx.Rect{X: x + (dotW-sz)/2, Y: r.Y + (r.H-sz)/2, W: sz, H: sz}, sz/2, d.col)
		if hot {
			widgets.Tooltip(ui, id, p.preview(d))
		}
		if ui.Clicked(id, dr) {
			p.jump(d)
		}
		if d.card > 0 && ui.RightClicked(id, dr) {
			p.undoTo = d.card
			ui.OpenPopup(p.undoMenu, ui.Mouse())
		}
		x += dotW
	}
	if widgets.ContextMenu(ui, p.undoMenu) {
		if widgets.MenuItemIcon(ui, i18n.Tr("editor.agent.undo_to_here", "Undo to Here"), "", "nav.back", true, false) {
			p.undoBack(p.undoTo)
		}
		widgets.EndContextMenu(ui)
	}
	_ = th
}

// preview is the tooltip of a dot: the step, and for a change its first changes.
func (p *Panel) preview(d *dot) string {
	text := d.label
	if c := p.card(d.card); c != nil {
		for i, r := range c.Rows {
			if i == 4 {
				text += "\n…"
				break
			}
			line := r.Title + ": " + r.Text
			if r.Before != "" || r.After != "" {
				line += " " + firstLine(r.Before) + " → " + firstLine(r.After)
			}
			text += "\n" + line
		}
	}
	return text
}

// jump scrolls the transcript to a dot's block, opens it and takes the editor to the place it changed.
func (p *Panel) jump(d *dot) {
	y := float32(0)
	for i := 0; i < d.block && i < len(p.blocks); i++ {
		y += p.blocks[i].h + p.gap
	}
	p.scroll, p.stick = y, false
	if d.block < len(p.blocks) {
		b := &p.blocks[d.block]
		b.open = true
		if d.step < len(b.steps) {
			b.steps[d.step].open = b.steps[d.step].look.Detail.Kind != agentui.DetailNone
		}
		b.dirty()
		if f := p.d.Follow; f != nil && d.step < len(b.steps) {
			f.Go(b.steps[d.step].tool, b.steps[d.step].args)
		}
	}
}

// undoBack undoes the applied cards from the newest back to card, stopping where one is no longer the last step.
func (p *Panel) undoBack(card int) {
	for i := len(p.cards) - 1; i >= 0; i-- {
		c := &p.cards[i]
		if c.State != agentui.Applied {
			continue
		}
		if !c.CanUndo {
			return
		}
		if err := p.d.Host.Undo(c.ID); err != nil {
			return
		}
		if c.ID == card {
			return
		}
		p.cards = p.d.Host.Cards(p.cards[:0])
	}
}

// where is the row under the header: where the agent is now, and Resume while following is paused.
func (p *Panel) where(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	f := p.d.Follow
	if f == nil {
		return
	}
	ui.Rect(r, th.PanelAlt)
	x := r.X + th.Padding
	right := r.X + r.W - th.Padding
	if f.Paused() {
		label := i18n.Tr("editor.agent.resume", "Resume Following")
		bw := ui.TextSize(label).X + 20
		if widgets.ButtonRect(ui, ui.ID("resume"), mathx.Rect{X: right - bw, Y: r.Y + 2, W: bw, H: r.H - 4}, label) {
			f.ResumeNow()
		}
		right -= bw + 6
	}
	sp, at, ok := f.Model.Where(follow.Agent, p.now())
	if !ok {
		text := i18n.Tr("editor.agent.following", "Following the agent")
		if f.Paused() {
			text = i18n.Tr("editor.agent.following_paused", "Following paused while you work")
		}
		ui.Text(p.fits[3].get(ui, text, right-x), mathx.Vec2{X: x, Y: ui.TextY(r)}, th.TextDim)
		return
	}
	wpresence.Marker(ui, mathx.Rect{X: x, Y: r.Y + (r.H-14)/2, W: 14, H: 14}, p.now()-at)
	x += 20
	label := p.spotLabel(sp)
	lr := mathx.Rect{X: x, Y: r.Y, W: min(ui.TextSize(label).X+4, right-x), H: r.H}
	id := ui.ID("where")
	if ui.Clicked(id, lr) {
		f.GoTo(sp)
	}
	col := th.Text
	if ui.Hot() == id {
		col = th.Accent
	}
	ui.Text(p.fits[3].get(ui, label, right-x), mathx.Vec2{X: x, Y: ui.TextY(r)}, col)
	widgets.Tooltip(ui, id, i18n.Tr("editor.agent.where_tip", "Where the agent works now; click to go there"))
}

// spotLabel is a place as the user names it: the file, then the node and the component, or the line.
func (p *Panel) spotLabel(sp presence.Spot) string {
	label := docName(sp.Doc)
	switch {
	case sp.Line > 0:
		return label + ":" + itoa(sp.Line)
	case sp.Panel == "console":
		return i18n.Tr("editor.agent.where_console", "Console")
	case sp.Panel == "problems":
		return i18n.Tr("editor.agent.where_problems", "Problems")
	}
	if !sp.Node.IsZero() && p.d.Docs != nil {
		if d := p.d.Docs.Documents().Find(sp.Doc); d != nil {
			s := d.Scene()
			if h := s.Handle(sp.Node); !h.IsZero() && s.IsAlive(h) {
				label += " › " + s.Name(h)
			}
		}
	}
	if sp.Type != "" {
		label += " › " + path.Ext("." + sp.Type)[1:]
	}
	return label
}

func (p *Panel) now() float64 {
	if p.d.Seconds != nil {
		return p.d.Seconds()
	}
	return 0
}

// docName is the file part of a document path.
func docName(rel string) string {
	return path.Base(rel)
}
