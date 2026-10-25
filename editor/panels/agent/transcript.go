package agent

import (
	"strings"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/richtext"
	"aego/ui/widgets"
)

// The transcript is virtual: every block has a height measured once for the panel width, and only the blocks in view are laid out and drawn (11b5a).

func (p *Panel) list() []block {
	if p.viewing != "" {
		return p.old
	}
	return p.blocks
}

func (p *Panel) transcript(ui *core.Context, body, track mathx.Rect) {
	th := &ui.Theme
	list := p.list()
	width := max(40, body.W-2*th.Padding)
	gap := th.Spacing * 2
	total := th.Padding
	for i := range list {
		b := &list[i]
		if b.hw != width {
			b.h, b.hw = p.measure(ui, b, width), width
		}
		total += b.h + gap
	}
	busy := p.viewing == "" && (p.d.Host.Busy() || p.waiting())
	if busy {
		total += th.RowHeight
	}
	if len(list) == 0 {
		total = body.H
	}
	total += th.Padding
	view := body.H
	maxOff := max(0, total-view)
	if ui.MouseOver(ui.ID("scroll"), body) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
			p.stick = p.scroll >= maxOff-1
		}
	}
	if p.stick {
		p.scroll = maxOff
	}
	p.scroll = mathx.Clamp(p.scroll, 0, maxOff)
	ui.PushClip(body)
	if len(list) == 0 && p.viewing == "" {
		p.empty(ui, body)
	}
	y := body.Y + th.Padding - p.scroll
	for i := range list {
		b := &list[i]
		if y+b.h >= body.Y && y <= body.Y+body.H {
			ui.PushIDInt(i)
			p.draw(ui, b, mathx.Rect{X: body.X + th.Padding, Y: y, W: width, H: b.h})
			ui.PopID()
		}
		y += b.h + gap
	}
	if busy {
		p.working(ui, mathx.Rect{X: body.X + th.Padding, Y: y, W: width, H: th.RowHeight})
	}
	ui.PopClip()
	p.content = total
	before := p.scroll
	widgets.ScrollBar(ui, ui.ID("vbar"), track, &p.scroll, total, view, &p.bar)
	if p.scroll != before {
		p.stick = p.scroll >= maxOff-1
	}
}

// measure is a block's height for a width; draw lays the block out the same way.
func (p *Panel) measure(ui *core.Context, b *block, w float32) float32 {
	th := &ui.Theme
	row := th.RowHeight
	switch b.kind {
	case blockUser:
		bw := bubbleWidth(ui, b.text, w)
		b.lines = widgets.WrapLines(ui, b.text, bw-2*th.Padding-4)
		return float32(len(b.lines))*ui.LineHeight() + 2*th.Padding + 2
	case blockText:
		p.parse(b)
		return b.md.Measure(ui, w)
	case blockTurn:
		return row * 0.9
	case blockError:
		return row
	case blockSteps:
		if len(b.steps) == 1 {
			return row + stepDetail(ui, &b.steps[0], w-18)
		}
		h := row
		if b.open {
			for i := range b.steps {
				h += row + stepDetail(ui, &b.steps[i], w-36)
			}
		}
		return h
	case blockFailed:
		h := 3*row + th.Padding*2
		if b.open {
			h += detailHeight(ui, &b.steps[0].look.Detail, w-18, false)
		}
		return h
	case blockCard:
		h := row
		if p.applyStrip(b) {
			h += row + th.Spacing
		}
		if b.open {
			h += p.cardBody(ui, b, w, nil)
		}
		return h
	case blockQuestion:
		return p.questionBody(ui, b, mathx.Rect{W: w}, false)
	}
	return row
}

func (p *Panel) parse(b *block) {
	if b.md == nil {
		b.md = &richtext.Text{Highlight: p.d.Highlight}
	}
	if b.parsed != b.text {
		b.md.Set(richtext.Parse(b.text))
		b.parsed = b.text
	}
}

func bubbleWidth(ui *core.Context, text string, w float32) float32 {
	th := &ui.Theme
	return min(w*0.85, ui.TextSize(text).X+2*th.Padding+8)
}

func (p *Panel) draw(ui *core.Context, b *block, r mathx.Rect) {
	th := &ui.Theme
	switch b.kind {
	case blockUser:
		bw := bubbleWidth(ui, b.text, r.W)
		box := mathx.Rect{X: r.X + r.W - bw, Y: r.Y, W: bw, H: r.H - 2}
		ui.RoundRect(box, 8, th.Agent.User)
		y := box.Y + th.Padding
		for _, line := range b.lines {
			mentions(ui, line, mathx.Vec2{X: box.X + th.Padding + 2, Y: y})
			y += ui.LineHeight()
		}
	case blockText:
		p.parse(b)
		if act := b.md.Draw(ui, ui.ID("md"), r); act.Target != "" {
			p.openLink(act.Target)
		}
	case blockTurn:
		mid := r.Y + r.H/2
		label := b.at.Format("15:04")
		lw := ui.TextSize(label).X + 12
		ui.Rect(mathx.Rect{X: r.X, Y: mid, W: (r.W - lw) / 2, H: 1}, th.Border)
		ui.Rect(mathx.Rect{X: r.X + (r.W+lw)/2, Y: mid, W: (r.W - lw) / 2, H: 1}, th.Border)
		ui.TextIn(r, label, th.TextDisabled, core.AlignCenter)
	case blockError:
		widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "status.error", th.Error)
		id := ui.ID("err")
		ui.MouseOver(id, r)
		ui.Text(widgets.Elide(ui, b.text, r.W-22), mathx.Vec2{X: r.X + 22, Y: ui.TextY(r)}, th.Error)
		widgets.Tooltip(ui, id, b.text)
	case blockSteps:
		p.drawSteps(ui, b, r)
	case blockFailed:
		p.drawFailed(ui, b, r)
	case blockCard:
		p.drawCard(ui, b, r)
	case blockQuestion:
		p.questionBody(ui, b, r, true)
	}
}

// mentions draws a line of the user's message with @ references as chips.
func mentions(ui *core.Context, line string, at mathx.Vec2) {
	th := &ui.Theme
	x := at.X
	for len(line) > 0 {
		i := strings.IndexByte(line, '@')
		if i < 0 {
			ui.Text(line, mathx.Vec2{X: x, Y: at.Y}, th.Text)
			return
		}
		if i > 0 {
			ui.Text(line[:i], mathx.Vec2{X: x, Y: at.Y}, th.Text)
			x += ui.TextSize(line[:i]).X
		}
		j := strings.IndexByte(line[i:], ' ')
		if j < 0 {
			j = len(line) - i
		}
		tok := line[i : i+j]
		w := ui.TextSize(tok).X
		ui.RoundRect(mathx.Rect{X: x - 2, Y: at.Y, W: w + 4, H: ui.LineHeight()}, 3, th.AccentDim)
		ui.Text(tok, mathx.Vec2{X: x, Y: at.Y}, th.Text)
		x += w
		line = line[i+j:]
	}
}

func (p *Panel) openLink(target string) {
	if p.d.Open != nil {
		p.d.Open(target)
	}
}

// statusIcon is the right edge of a step row: busy, done or failed.
func statusIcon(ui *core.Context, r mathx.Rect, s *step) {
	th := &ui.Theme
	ir := mathx.Rect{X: r.X + r.W - 16, Y: r.Y + (r.H-14)/2, W: 14, H: 14}
	switch {
	case !s.done:
		col := th.TextDim
		col.A *= 0.5 + 0.5*mathx.Sin(float32(ui.Time)*6)
		widgets.IconAt(ui, ir, "status.busy", col)
	case s.failed:
		widgets.IconAt(ui, ir, "status.error", th.Error)
	default:
		col := th.Ok
		col.A = 0.75
		widgets.IconAt(ui, ir, "status.ok", col)
	}
}

// stepRow is one call: icon, title · summary and its state; a click opens what it found and reports true.
func (p *Panel) stepRow(ui *core.Context, s *step, r mathx.Rect, key int) bool {
	th := &ui.Theme
	id := ui.IDInt(1000 + key)
	has := s.look.Detail.Kind != agentui.DetailNone
	toggled := has && ui.Clicked(id, r)
	if toggled {
		s.open = !s.open
	}
	if ui.Hot() == id {
		ui.RoundRect(r, th.Radius.Button, th.WidgetHot)
	}
	x := r.X + 2
	if has {
		widgets.Chevron(ui, mathx.Rect{X: x, Y: r.Y, W: 14, H: r.H}, s.open, th.TextDim)
	}
	x += 16
	widgets.IconAt(ui, mathx.Rect{X: x, Y: r.Y + (r.H-14)/2, W: 14, H: 14}, s.look.Icon, th.TextDim)
	x += 20
	ui.Text(s.fl.get(ui, s.label, max(0, r.X+r.W-x-22)), mathx.Vec2{X: x, Y: ui.TextY(r)}, th.TextDim)
	statusIcon(ui, r, s)
	return toggled
}

func stepDetail(ui *core.Context, s *step, w float32) float32 {
	if !s.open {
		return 0
	}
	return detailHeight(ui, &s.look.Detail, w, s.all)
}

func (p *Panel) drawSteps(ui *core.Context, b *block, r mathx.Rect) {
	th := &ui.Theme
	row := th.RowHeight
	ui.RoundRect(r, th.Radius.Button, th.Agent.Tool)
	if len(b.steps) == 1 {
		s := &b.steps[0]
		hr := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: row}
		if p.stepRow(ui, s, hr, 0) {
			b.dirty()
		}
		if s.open && p.drawDetail(ui, s, mathx.Rect{X: r.X + 18, Y: r.Y + row, W: r.W - 18}) {
			b.dirty()
		}
		return
	}
	hr := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: row}
	id := ui.ID("group")
	if ui.Clicked(id, hr) {
		b.open = !b.open
		b.dirty()
	}
	if ui.Hot() == id {
		ui.RoundRect(hr, th.Radius.Button, th.WidgetHot)
	}
	widgets.Chevron(ui, mathx.Rect{X: hr.X + 2, Y: hr.Y, W: 14, H: hr.H}, b.open, th.TextDim)
	x := hr.X + 18
	shown := 0
	for i := range b.steps {
		icon := b.steps[i].look.Icon
		dup := false
		for j := 0; j < i; j++ {
			if b.steps[j].look.Icon == icon {
				dup = true
				break
			}
		}
		if dup || shown == 6 {
			continue
		}
		widgets.IconAt(ui, mathx.Rect{X: x, Y: hr.Y + (hr.H-14)/2, W: 14, H: 14}, icon, th.TextDim)
		x += 17
		shown++
	}
	x += 4
	if b.text == "" || b.hw < 0 {
		b.text = i18n.TrN("editor.agent.looked_around", int64(len(b.steps)), "Looked around · {n} step", "Looked around · {n} steps")
	}
	ui.Text(b.fl.get(ui, b.text, max(0, hr.X+hr.W-x-22)), mathx.Vec2{X: x, Y: ui.TextY(hr)}, th.TextDim)
	last := &b.steps[len(b.steps)-1]
	statusIcon(ui, hr, last)
	if !b.open {
		return
	}
	y := r.Y + row
	for i := range b.steps {
		s := &b.steps[i]
		if p.stepRow(ui, s, mathx.Rect{X: r.X + 18, Y: y, W: r.W - 18, H: row}, i) {
			b.dirty()
		}
		y += row
		if s.open {
			if p.drawDetail(ui, s, mathx.Rect{X: r.X + 36, Y: y, W: r.W - 36}) {
				b.dirty()
			}
			y += detailHeight(ui, &s.look.Detail, r.W-36, s.all)
		}
	}
}

func (p *Panel) drawFailed(ui *core.Context, b *block, r mathx.Rect) {
	th := &ui.Theme
	row := th.RowHeight
	s := &b.steps[0]
	ui.RoundRect(r, th.Radius.Button, th.Agent.Tool)
	ui.Rect(mathx.Rect{X: r.X, Y: r.Y, W: 2, H: r.H}, th.Error)
	hr := mathx.Rect{X: r.X + th.Padding, Y: r.Y + th.Padding, W: r.W - 2*th.Padding, H: row}
	widgets.IconAt(ui, mathx.Rect{X: hr.X, Y: hr.Y + (row-14)/2, W: 14, H: 14}, "status.error", th.Error)
	title := i18n.Tr("editor.agent.failed", "Failed: {title}", i18n.Str("title", s.look.Title))
	ui.Text(widgets.Elide(ui, title, hr.W-20), mathx.Vec2{X: hr.X + 20, Y: ui.TextY(hr)}, th.Text)
	er := mathx.Rect{X: hr.X + 20, Y: hr.Y + row, W: hr.W - 20, H: row}
	eid := ui.ID("why")
	if ui.Clicked(eid, er) {
		b.open = !b.open
		b.dirty()
	}
	ui.Text(widgets.Elide(ui, s.look.Summary, er.W), mathx.Vec2{X: er.X, Y: ui.TextY(er)}, th.TextDim)
	widgets.Tooltip(ui, eid, s.look.Summary)
	y := er.Y + row
	if b.open {
		p.drawDetail(ui, s, mathx.Rect{X: r.X + 18, Y: y, W: r.W - 18})
		y += detailHeight(ui, &s.look.Detail, r.W-18, false)
	}
	bx := hr.X + 20
	button := func(key, label string) bool {
		w := ui.TextSize(label).X + 20
		hit := widgets.ButtonRect(ui, ui.ID(key), mathx.Rect{X: bx, Y: y, W: w, H: row}, label)
		bx += w + 6
		return hit
	}
	if button("retry", i18n.Tr("editor.agent.retry", "Retry")) {
		p.sendText(i18n.Tr("editor.agent.retry_text", "Try the step that failed again: {title}", i18n.Str("title", s.look.Title)))
	}
	if button("fix", i18n.Tr("editor.agent.ask_fix", "Ask Agent to Fix")) {
		p.sendText(i18n.Tr("editor.agent.fix_text", "The step “{title}” failed with: {error}. Find the cause and fix it.", i18n.Str("title", s.look.Title), i18n.Str("error", s.look.Summary)))
	}
}

// working is the last row while the agent answers: the current step's icon pulses next to what it does.
func (p *Panel) working(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	icon, label := "agent.avatar", i18n.Tr("editor.agent.thinking", "Thinking…")
	switch s := p.running(); {
	case p.waiting():
		icon, label = "status.warning", i18n.Tr("editor.agent.waiting", "Waiting for you")
	case s != nil && s.look.Write:
		icon, label = s.look.Icon, i18n.Tr("editor.agent.preparing", "Preparing a change…")
	case s != nil:
		icon, label = s.look.Icon, i18n.Tr("editor.agent.reading", "Reading the project…")
	}
	col := th.Agent.Presence
	col.A = 0.45 + 0.55*mathx.Abs(mathx.Sin(float32(ui.Time)*3))
	widgets.IconAt(ui, mathx.Rect{X: r.X + 2, Y: r.Y + (r.H-14)/2, W: 14, H: 14}, icon, col)
	ui.Text(label, mathx.Vec2{X: r.X + 22, Y: ui.TextY(r)}, th.TextDim)
}

// empty invites the first message: what to ask and three examples that send themselves.
func (p *Panel) empty(ui *core.Context, body mathx.Rect) {
	th := &ui.Theme
	w := min(body.W-4*th.Padding, 420)
	x := body.X + (body.W-w)/2
	y := body.Y + max(th.Padding*2, body.H*0.18)
	widgets.IconAt(ui, mathx.Rect{X: x + (w-24)/2, Y: y, W: 24, H: 24}, "agent.avatar", th.Agent.Presence)
	y += 32
	ui.TextIn(mathx.Rect{X: x, Y: y, W: w, H: th.RowHeight}, i18n.Tr("editor.agent.empty_title", "Ask the agent"), th.Text, core.AlignCenter)
	y += th.RowHeight
	hint := i18n.Tr("editor.agent.empty_hint", "Ask about the project or describe a change. Type @ to mention a node or an asset. Changes wait for you to apply them.")
	for _, line := range widgets.WrapLines(ui, hint, w) {
		ui.TextIn(mathx.Rect{X: x, Y: y, W: w, H: ui.LineHeight()}, line, th.TextDim, core.AlignCenter)
		y += ui.LineHeight()
	}
	y += th.Spacing * 3
	examples := [3]string{
		i18n.Tr("editor.agent.example_platform", "Add a moving platform above the player"),
		i18n.Tr("editor.agent.example_explain", "Explain what this scene does"),
		i18n.Tr("editor.agent.example_enemy", "Find why the enemy doesn't die"),
	}
	for i, ex := range examples {
		er := mathx.Rect{X: x, Y: y, W: w, H: th.RowHeight + 6}
		id := ui.IDInt(500 + i)
		if ui.Clicked(id, er) {
			p.sendText(ex)
		}
		fill := th.Agent.Tool
		if ui.Hot() == id {
			fill = th.WidgetHot
		}
		ui.RoundRect(er, th.Radius.Button, fill)
		widgets.IconAt(ui, mathx.Rect{X: er.X + 8, Y: er.Y + (er.H-14)/2, W: 14, H: 14}, "agent.new", th.TextDim)
		ui.Text(widgets.Elide(ui, ex, er.W-36), mathx.Vec2{X: er.X + 30, Y: ui.TextY(er)}, th.Text)
		y += er.H + th.Spacing
	}
}

// keyDigit is the 1–9 key pressed this frame, 0 when none.
func keyDigit(ui *core.Context) int {
	for i := 0; i < 9; i++ {
		if ui.KeyPressed(input.Key1 + input.Scancode(i)) {
			return i + 1
		}
	}
	return 0
}
