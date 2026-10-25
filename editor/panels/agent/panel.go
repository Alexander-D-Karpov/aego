// Package agent is the editor panel that talks to the coding agent and shows its proposals (D343).
package agent

import (
	"path"
	"strings"
	"time"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/richtext"
	"aego/ui/widgets"
	"aego/ui/widgets/palette"
)

const (
	ID           = "agent"
	Icon         = "panel.agent"
	maxCardLines = 8
	maxResult    = 4000
	cardInset    = 8
	installHelp  = "The agent runs Claude Code, the claude command. Install it, sign in once by running claude in a terminal, then check again. If it is installed where the editor does not look, set its path below."
	emptyHint    = "Ask about the project or describe a change. Type @ to mention a node or an asset. Changes wait for you to apply them."
)

type itemKind uint8

const (
	itemUser itemKind = iota
	itemText
	itemTool
	itemCard
	itemError
)

type item struct {
	kind   itemKind
	text   string
	md     *richtext.Text
	parsed string
	h      float32
	tool   string
	call   string
	args   string
	result string
	failed bool
	done   bool
	open   bool
	card   int
	state  agentui.State
}

type Deps struct {
	Host    agentui.Host
	Docs    services.Documents
	Notices services.Notices
	// Assets lists the project's asset paths for @ mentions.
	Assets func() []string
	Root   string
	Show   func()
	// Highlight colours code blocks in answers, the same as the documentation panel.
	Highlight richtext.Highlighter
}

type avail struct {
	version string
	err     error
}

type Panel struct {
	d        Deps
	items    []item
	old      []item
	events   []agentui.Event
	cards    []agentui.Card
	seen     map[int]bool
	logged   map[int]agentui.State
	input    string
	prevLen  int
	scroll   float32
	content  float32
	bar      widgets.ScrollState
	follow   bool
	focus    bool
	ver      string
	verErr   error
	checking bool
	checked  chan avail
	path     string
	log      *agentui.Transcript
	logErr   bool
	past     []string
	names    []string
	pastSel  int
	viewing  string
	pal      *palette.Palette
	mentions []string
	notified bool
	allN     int
	allLabel string
	turnAll  bool
}

func New(d Deps) *Panel {
	return &Panel{d: d, seen: map[int]bool{}, logged: map[int]agentui.State{}, checked: make(chan avail, 1), follow: true}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	for i := range p.cards {
		if p.cards[i].State == agentui.Pending {
			return i18n.Tr("editor.agent.agent", "Agent •")
		}
	}
	return i18n.Tr("editor.agent.agent_2", "Agent")
}

// Missing is why Claude Code cannot run, nil when it can or while checking.
func (p *Panel) Missing() error {
	return p.verErr
}

func (p *Panel) Input() string {
	return p.input
}

// Recheck looks for Claude Code again on the next Tick.
func (p *Panel) Recheck() {
	p.ver, p.verErr = "", nil
}

// Ask shows the panel with the prompt in the input, not sent.
func (p *Panel) Ask(prompt string) {
	p.viewing, p.pastSel = "", 0
	if prompt != "" {
		p.input = prompt
		p.prevLen = len(prompt)
	}
	p.focus = true
	if p.d.Show != nil {
		p.d.Show()
	}
}

// Submit sends text as if typed and sent, for tests.
func (p *Panel) Submit(text string) {
	p.input = text
	p.send()
}

func (p *Panel) Pending() []int {
	var out []int
	for i := range p.cards {
		if p.cards[i].State == agentui.Pending {
			out = append(out, p.cards[i].ID)
		}
	}
	return out
}

func (p *Panel) Decide(id int, apply bool) {
	p.d.Host.Decide(id, apply)
}

func (p *Panel) Close() {
	p.d.Host.Close()
	if p.log != nil {
		p.log.Close()
		p.log = nil
	}
}

func (p *Panel) check() {
	p.checking, p.verErr = true, nil
	host, cmd, ch := p.d.Host, p.d.Host.Command(), p.checked
	go func() {
		v, err := host.Available(cmd)
		ch <- avail{version: v, err: err}
	}()
}

func (p *Panel) record(e agentui.Entry) {
	if p.log == nil {
		return
	}
	if err := p.log.Append(e); err != nil && !p.logErr {
		p.logErr = true
		p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.agent.cannot_write_the_agent_session", "cannot write the agent session log: {error}", i18n.Str("error", err.Error())))
	}
}

func (p *Panel) add(it item) {
	p.items = append(p.items, it)
}

// Tick takes what the model and the bridge produced since the last frame; it runs also while the panel is hidden.
func (p *Panel) Tick() {
	select {
	case a := <-p.checked:
		p.ver, p.verErr, p.checking = a.version, a.err, false
	default:
	}
	if p.ver == "" && p.verErr == nil && !p.checking {
		p.check()
	}
	host := p.d.Host
	p.events = host.Poll(p.events[:0])
	for _, e := range p.events {
		p.event(e)
	}
	p.cards = host.Cards(p.cards[:0])
	pending := false
	for i := range p.cards {
		c := &p.cards[i]
		if !p.seen[c.ID] {
			p.seen[c.ID] = true
			p.add(item{kind: itemCard, card: c.ID})
			p.follow = true
		}
		if c.State == agentui.Pending && p.turnAll && !c.Destructive {
			p.d.Host.Decide(c.ID, true)
			c.State = agentui.Approved
		}
		if c.State == agentui.Pending {
			pending = true
		}
		if st, ok := p.logged[c.ID]; !ok || st != c.State {
			p.logged[c.ID] = c.State
			p.record(agentui.Entry{Kind: "card", Card: c.ID, Text: summary(c), State: c.State.String()})
		}
	}
	switch {
	case pending && !p.notified:
		p.notified = true
		if p.d.Show != nil {
			p.d.Show()
		}
		p.d.Notices.Notify(context.LevelInfo, i18n.Note("editor.agent.the_agent_is_waiting_for", "The agent is waiting for you to apply or reject a change"))
	case !pending:
		p.notified = false
	}
}

func (p *Panel) event(e agentui.Event) {
	switch e.Kind {
	case agentui.EvText:
		p.text(&p.items, e.Text)
		p.record(agentui.Entry{Kind: "text", Text: e.Text})
	case agentui.EvToolStart:
		p.add(item{kind: itemTool, tool: e.Tool, call: e.CallID, args: e.Args})
		p.record(agentui.Entry{Kind: "tool", Tool: e.Tool, CallID: e.CallID, Args: e.Args})
	case agentui.EvToolResult:
		finish(p.items, e.Tool, e.CallID, e.Result, e.Failed)
		p.record(agentui.Entry{Kind: "result", Tool: e.Tool, CallID: e.CallID, Text: e.Result, Failed: e.Failed})
	case agentui.EvError:
		p.add(item{kind: itemError, text: e.Text})
		p.record(agentui.Entry{Kind: "error", Text: e.Text})
	}
	if e.Kind == agentui.EvTurnEnd || e.Kind == agentui.EvError {
		p.turnAll = false
	}
	p.follow = p.follow || e.Kind != agentui.EvTurnEnd
}

// text joins answer text into one message until the next tool call.
func (p *Panel) text(list *[]item, s string) {
	if n := len(*list); n > 0 && (*list)[n-1].kind == itemText {
		(*list)[n-1].text += "\n\n" + s
		return
	}
	*list = append(*list, item{kind: itemText, text: s})
}

func finish(list []item, tool, call, result string, failed bool) {
	for i := len(list) - 1; i >= 0; i-- {
		it := &list[i]
		if it.kind != itemTool || it.done {
			continue
		}
		if (call != "" && it.call == call) || (call == "" && it.tool == tool) {
			it.result, it.failed, it.done = result, failed, true
			return
		}
	}
}

func (p *Panel) send() {
	text := strings.TrimSpace(p.input)
	if text == "" || p.d.Host.Busy() || p.viewing != "" {
		return
	}
	if p.log == nil {
		t, err := agentui.NewTranscript(p.d.Root, time.Now())
		if err != nil {
			p.d.Notices.Notify(context.LevelWarn, i18n.Note("editor.agent.cannot_start_the_agent_session", "cannot start the agent session log: {error}", i18n.Str("error", err.Error())))
		} else {
			p.log, p.past = t, nil
		}
	}
	p.add(item{kind: itemUser, text: text})
	p.record(agentui.Entry{Kind: "user", Text: text})
	p.input, p.prevLen, p.follow = "", 0, true
	if err := p.d.Host.Send(text); err != nil {
		p.add(item{kind: itemError, text: err.Error()})
		p.record(agentui.Entry{Kind: "error", Text: err.Error()})
	}
}

func summary(c *agentui.Card) string {
	lines := []string{c.Command}
	for _, ch := range c.Changes {
		lines = append(lines, ch.String())
	}
	lines = append(lines, c.Notes...)
	return strings.Join(lines, "\n")
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.verErr != nil && !p.d.Host.Running() {
		p.drawMissing(ui)
		return
	}
	bar := ui.Cursor().Next(0)
	p.header(ui, bar)
	top := bar.Y + bar.H + th.Spacing
	inputH := th.RowHeight + 2*th.Padding
	body := mathx.Rect{X: r.X, Y: top, W: max(0, r.W-th.ScrollbarW-2), H: max(0, r.Y+r.H-top-inputH)}
	p.transcript(ui, body, mathx.Rect{X: body.X + body.W + 2, Y: body.Y, W: th.ScrollbarW, H: body.H})
	p.inputRow(ui, mathx.Rect{X: r.X + th.Padding, Y: body.Y + body.H + th.Padding, W: max(0, r.W-2*th.Padding), H: th.RowHeight})
	p.drawMentions(ui)
}

func (p *Panel) header(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	host := p.d.Host
	if p.past == nil {
		p.past, _ = agentui.List(p.d.Root)
		if p.past == nil {
			p.past = []string{}
		}
		p.names = append(p.names[:0], "Current session")
		for _, id := range p.past {
			if p.log != nil && id == p.log.ID {
				continue
			}
			p.names = append(p.names, id)
		}
	}
	cw := min(float32(170), r.W*0.4)
	sel := p.pastSel
	if widgets.Combo(ui, ui.ID("sessions"), mathx.Rect{X: r.X, Y: r.Y, W: cw, H: r.H}, &sel, p.names, false) && sel != p.pastSel {
		p.open(sel, p.names)
	}
	x := r.X + cw + 6
	auto := host.AutoApply()
	aw := ui.TextSize(i18n.Tr("editor.agent.auto_apply", "Auto-apply")).X + 20
	if widgets.ToggleButton(ui, ui.ID("auto"), mathx.Rect{X: x, Y: r.Y, W: aw, H: r.H}, i18n.Tr("editor.agent.auto_apply", "Auto-apply"), &auto) {
		host.SetAutoApply(auto)
	}
	widgets.Tooltip(ui, ui.ID("auto"), i18n.Tr("editor.agent.apply_changes_without_asking_removals", "Apply changes without asking; removals, moves and deletes still wait for you"))
	x += aw + 6
	if n := p.applicable(); n >= 1 && host.Busy() {
		if n != p.allN {
			p.allN, p.allLabel = n, "Apply All"
			if n > 1 {
				p.allLabel = "Apply All (" + itoa(n) + ")"
			}
		}
		bw := ui.TextSize(p.allLabel).X + 20
		if widgets.ButtonRect(ui, ui.ID("applyall"), mathx.Rect{X: x, Y: r.Y, W: bw, H: r.H}, p.allLabel) {
			p.ApplyAll()
		}
		widgets.Tooltip(ui, ui.ID("applyall"), i18n.Tr("editor.agent.apply_this_answer_s_changes", "Apply this answer's changes, the waiting ones and those still to come; removals, moves and deletes still wait for you"))
		x += bw + 6
	}
	status, col := p.ver, th.TextDisabled
	if host.Busy() {
		sw := ui.TextSize(i18n.Tr("editor.agent.stop", "Stop")).X + 20
		if widgets.ButtonRect(ui, ui.ID("stop"), mathx.Rect{X: r.X + r.W - sw, Y: r.Y, W: sw, H: r.H}, i18n.Tr("editor.agent.stop", "Stop")) {
			host.Cancel()
		}
		status, col = i18n.Tr("editor.agent.working", "Working…"), th.TextDim
		r.W -= sw + 6
	}
	ui.Text(widgets.Elide(ui, status, max(0, r.X+r.W-x)), mathx.Vec2{X: x, Y: ui.TextY(r)}, col)
}

func (p *Panel) open(sel int, names []string) {
	p.pastSel, p.scroll = sel, 0
	if sel == 0 {
		p.viewing, p.old, p.follow = "", nil, true
		return
	}
	id := names[sel]
	entries, err := agentui.Load(p.d.Root, id)
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		p.pastSel = 0
		return
	}
	p.viewing, p.old, p.follow = id, itemsFrom(entries), false
}

func itemsFrom(entries []agentui.Entry) []item {
	var out []item
	cards := map[int]int{}
	for _, e := range entries {
		switch e.Kind {
		case "user":
			out = append(out, item{kind: itemUser, text: e.Text})
		case "text":
			if n := len(out); n > 0 && out[n-1].kind == itemText {
				out[n-1].text += "\n\n" + e.Text
			} else {
				out = append(out, item{kind: itemText, text: e.Text})
			}
		case "tool":
			out = append(out, item{kind: itemTool, tool: e.Tool, call: e.CallID, args: e.Args})
		case "result":
			finish(out, e.Tool, e.CallID, e.Text, e.Failed)
		case "card":
			if i, ok := cards[e.Card]; ok {
				out[i].text, out[i].state = e.Text, agentui.ParseState(e.State)
				continue
			}
			cards[e.Card] = len(out)
			out = append(out, item{kind: itemCard, card: e.Card, text: e.Text, state: agentui.ParseState(e.State)})
		case "error":
			out = append(out, item{kind: itemError, text: e.Text})
		}
	}
	return out
}

func (p *Panel) transcript(ui *core.Context, body, track mathx.Rect) {
	th := &ui.Theme
	view := body.H
	bottom := p.scroll >= p.content-view-2
	if ui.MouseOver(ui.ID("scroll"), body) {
		if w := ui.ScrollY(th.RowHeight * 3); w != 0 {
			p.scroll -= w
			p.follow = false
		}
	}
	list := p.items
	if p.viewing != "" {
		list = p.old
	}
	ui.PushClip(body)
	ui.PushCursor(core.Cursor{X: body.X + th.Padding, Y: body.Y + th.Padding - p.scroll, W: max(0, body.W-2*th.Padding), RowH: th.RowHeight, Pad: th.Padding, Space: th.Spacing})
	start := ui.Cursor().Y
	if len(list) == 0 {
		widgets.LabelWrapped(ui, emptyHint, true)
	}
	for i := range list {
		ui.PushIDInt(i)
		p.drawItem(ui, &list[i], body)
		ui.PopID()
	}
	p.content = ui.Cursor().Y - start + 2*th.Padding
	ui.PopCursor()
	ui.PopClip()
	maxOff := max(0, p.content-view)
	if p.follow || bottom {
		p.scroll = maxOff
	}
	p.scroll = mathx.Clamp(p.scroll, 0, maxOff)
	widgets.ScrollBar(ui, ui.ID("vbar"), track, &p.scroll, p.content, view, &p.bar)
}

func visible(r, body mathx.Rect) bool {
	return r.Y+r.H >= body.Y && r.Y <= body.Y+body.H
}

func (p *Panel) newText() *richtext.Text {
	return &richtext.Text{Highlight: p.d.Highlight}
}

func (p *Panel) drawItem(ui *core.Context, it *item, body mathx.Rect) {
	th := &ui.Theme
	switch it.kind {
	case itemUser:
		ui.Cursor().Next(4)
		widgets.LabelDim(ui, i18n.Tr("editor.agent.you", "You"))
		widgets.LabelWrapped(ui, it.text, false)
	case itemText:
		if it.md == nil {
			it.md = p.newText()
		}
		if it.parsed != it.text {
			it.md.Set(richtext.Parse(it.text))
			it.parsed = it.text
		}
		rr := ui.Cursor().Next(max(it.h, th.RowHeight))
		if visible(rr, body) {
			it.md.Draw(ui, ui.ID("md"), rr)
			if d := it.md.Doc(); d != nil {
				it.h = d.Height + 4*th.Padding
			}
		}
	case itemTool:
		p.drawTool(ui, it)
	case itemCard:
		p.drawCard(ui, it)
	case itemError:
		rr := ui.Cursor().Next(0)
		widgets.IconAt(ui, mathx.Rect{X: rr.X, Y: rr.Y + (rr.H-16)/2, W: 16, H: 16}, "log.error", th.Error)
		id := ui.ID("err")
		ui.MouseOver(id, rr)
		ui.Text(widgets.Elide(ui, it.text, rr.W-22), mathx.Vec2{X: rr.X + 22, Y: ui.TextY(rr)}, th.Error)
		widgets.Tooltip(ui, id, it.text)
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (p *Panel) drawTool(ui *core.Context, it *item) {
	th := &ui.Theme
	rr := ui.Cursor().Next(0)
	id := ui.ID("tool")
	if ui.Clicked(id, rr) {
		it.open = !it.open
	}
	if ui.Hot() == id {
		ui.Rect(rr, th.WidgetHot)
	}
	widgets.Chevron(ui, mathx.Rect{X: rr.X, Y: rr.Y, W: th.Indent, H: rr.H}, it.open, th.TextDim)
	status, col := "…", th.TextDim
	switch {
	case it.done && it.failed:
		status, col = "failed", th.Error
	case it.done:
		status = "ok"
	}
	sw := ui.TextSize(status).X + 8
	ui.Text(widgets.Elide(ui, it.tool+"  "+it.args, max(0, rr.W-th.Indent-sw-4)), mathx.Vec2{X: rr.X + th.Indent, Y: ui.TextY(rr)}, th.TextDim)
	ui.TextIn(rr, status, col, core.AlignRight)
	if !it.open {
		return
	}
	if it.args != "" {
		widgets.LabelWrapped(ui, it.args, true)
	}
	if it.result != "" {
		widgets.LabelWrapped(ui, clip(it.result, maxResult), true)
	}
}

func (p *Panel) card(id int) *agentui.Card {
	for i := range p.cards {
		if p.cards[i].ID == id {
			return &p.cards[i]
		}
	}
	return nil
}

func stateLabel(ui *core.Context, st agentui.State, err string) (string, mathx.Color) {
	th := &ui.Theme
	switch st {
	case agentui.Pending:
		return "waiting for you", th.Warn
	case agentui.Approved:
		return "applying…", th.TextDim
	case agentui.Applied:
		return "applied", th.Accent
	case agentui.Failed:
		return i18n.Tr("editor.agent.failed_err", "failed: {err}", i18n.Str("err", err)), th.Error
	}
	return st.String(), th.TextDisabled
}

func (p *Panel) drawCard(ui *core.Context, it *item) {
	th := &ui.Theme
	cur := ui.Cursor()
	start := cur.Y
	var c *agentui.Card
	if p.viewing == "" {
		c = p.card(it.card)
	}
	title, st, errText, lines, destructive := "", it.state, "", []string(nil), false
	if c != nil {
		title, st, errText, destructive = c.Command, c.State, c.Err, c.Destructive
		for _, ch := range c.Changes {
			lines = append(lines, ch.String())
		}
		lines = append(lines, c.Notes...)
		lines = append(lines, c.Problems...)
	} else {
		parts := strings.Split(it.text, "\n")
		title, lines = parts[0], parts[1:]
	}
	cur.Next(2)
	head := cur.Next(0)
	x := head.X + 6
	if destructive {
		widgets.IconAt(ui, mathx.Rect{X: x, Y: head.Y + (head.H-16)/2, W: 16, H: 16}, "status.warning", th.Warn)
		x += 20
	}
	label, col := stateLabel(ui, st, errText)
	lw := min(ui.TextSize(label).X+8, head.W/2)
	ui.Text(widgets.Elide(ui, title, max(0, head.X+head.W-lw-x)), mathx.Vec2{X: x, Y: ui.TextY(head)}, th.Text)
	ui.TextIn(mathx.Rect{X: head.X + head.W - lw, Y: head.Y, W: lw - 6, H: head.H}, widgets.Elide(ui, label, lw-6), col, core.AlignRight)
	cur.X, cur.W = cur.X+cardInset, cur.W-2*cardInset
	for i, l := range lines {
		if i == maxCardLines {
			widgets.LabelDim(ui, i18n.Tr("editor.agent.v_more", "+{v} more", i18n.Str("v", itoa(len(lines)-maxCardLines))))
			break
		}
		widgets.LabelWrapped(ui, l, true)
	}
	if c != nil && (c.State == agentui.Pending || c.CanUndo) {
		row := cur.Next(0)
		bx := row.X
		button := func(key, text string) bool {
			w := ui.TextSize(text).X + 20
			hit := widgets.ButtonRect(ui, ui.ID(key), mathx.Rect{X: bx, Y: row.Y, W: w, H: row.H}, text)
			bx += w + 6
			return hit
		}
		if c.State == agentui.Pending {
			if button("apply", i18n.Tr("editor.agent.apply", "Apply")) {
				p.d.Host.Decide(c.ID, true)
			}
			if button("reject", i18n.Tr("editor.agent.reject", "Reject")) {
				p.d.Host.Decide(c.ID, false)
			}
		}
		if c.CanUndo && button("undo", i18n.Tr("editor.agent.undo", "Undo")) {
			if err := p.d.Host.Undo(c.ID); err != nil {
				p.d.Notices.Notify(context.LevelWarn, err.Error())
			}
		}
	}
	cur.X, cur.W = cur.X-cardInset, cur.W+2*cardInset
	box := mathx.Rect{X: cur.X, Y: start, W: cur.W, H: cur.Y - start}
	ui.List().RectOutline(box, th.Border, 1)
	if st == agentui.Pending || destructive {
		stripe := th.Warn
		if st == agentui.Pending {
			stripe = th.Accent
		}
		ui.Rect(mathx.Rect{X: box.X, Y: box.Y, W: 2, H: box.H}, stripe)
	}
	cur.Next(2)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func (p *Panel) inputRow(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	host := p.d.Host
	locked := host.Busy() || p.viewing != ""
	bw := ui.TextSize(i18n.Tr("editor.agent.send", "Send")).X + 20
	id := ui.ID("message")
	if p.focus {
		ui.SetFocus(id)
		p.focus = false
	}
	fr := mathx.Rect{X: r.X, Y: r.Y, W: max(0, r.W-bw-4), H: r.H}
	enter := ui.Focus() == id && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter))
	widgets.TextFieldRect(ui, id, fr, &p.input, true)
	if p.input == "" && ui.Focus() != id {
		hint := i18n.Tr("editor.agent.message_the_agent_to_mention", "Message the agent, @ to mention")
		if p.viewing != "" {
			hint = "A past session is read-only, pick Current session to write"
		}
		ui.Text(widgets.Elide(ui, hint, fr.W-8), mathx.Vec2{X: fr.X + 4, Y: ui.TextY(fr)}, th.TextDisabled)
	}
	if len(p.input) > p.prevLen && strings.HasSuffix(p.input, "@") {
		p.input = strings.TrimSuffix(p.input, "@")
		p.openMentions(ui)
	}
	p.prevLen = len(p.input)
	sent := widgets.ButtonRect(ui, ui.ID("send"), mathx.Rect{X: r.X + r.W - bw, Y: r.Y, W: bw, H: r.H}, i18n.Tr("editor.agent.send", "Send"))
	if !locked && (sent || enter) {
		p.send()
		p.focus = true
	}
}

func (p *Panel) openMentions(ui *core.Context) {
	if p.pal == nil {
		p.pal = palette.New(core.IDOf("agent.mentions"))
		p.pal.Title = "Mention"
		p.pal.Hint = "Enter insert · Esc close"
	}
	var items []palette.Item
	p.mentions = p.mentions[:0]
	if d := p.d.Docs.Doc(); d != nil {
		s := d.Scene()
		root := s.Root()
		for _, h := range s.PreOrder() {
			if h == root || !s.IsAlive(h) {
				continue
			}
			id := s.ID(h)
			name := s.Name(h)
			items = append(items, palette.Item{Key: "node:" + id.String(), Label: name, Category: "Nodes", Detail: path.Base(d.Path), Icon: "cat.node"})
			p.mentions = append(p.mentions, "@"+name+" (#"+id.String()+" in "+d.Path+")")
		}
	}
	if p.d.Assets != nil {
		for _, a := range p.d.Assets() {
			items = append(items, palette.Item{Key: "asset:" + a, Label: path.Base(a), Category: "Assets", Detail: a})
			p.mentions = append(p.mentions, "@"+a)
		}
	}
	p.pal.SetItems(items)
	p.pal.Show(ui)
}

func (p *Panel) drawMentions(ui *core.Context) {
	if p.pal == nil || !p.pal.Active() {
		return
	}
	sz := ui.Size()
	w, h := min(560, sz.X-40), min(380, sz.Y-60)
	res := p.pal.Draw(ui, mathx.Rect{X: (sz.X - w) / 2, Y: max(20, sz.Y*0.15), W: w, H: h})
	switch {
	case res.Close:
		p.pal.Hide(ui)
		p.focus = true
	case res.Pick >= 0 && res.Pick < len(p.mentions):
		if p.input != "" && !strings.HasSuffix(p.input, " ") {
			p.input += " "
		}
		p.input += p.mentions[res.Pick] + " "
		p.prevLen = len(p.input)
		p.pal.Hide(ui)
		p.focus = true
	}
}

func (p *Panel) drawMissing(ui *core.Context) {
	widgets.Label(ui, i18n.Tr("editor.agent.claude_code_is_not_available", "Claude Code is not available"))
	widgets.LabelWrapped(ui, installHelp, true)
	if p.verErr != nil {
		widgets.LabelWrapped(ui, p.verErr.Error(), true)
	}
	if p.path == "" {
		p.path = p.d.Host.Command()
	}
	widgets.TextFieldRect(ui, ui.ID("path"), widgets.Field(ui, i18n.Tr("editor.agent.path", "Path")), &p.path, true)
	if p.checking {
		widgets.LabelDim(ui, i18n.Tr("editor.agent.checking", "Checking…"))
		return
	}
	if widgets.Button(ui, i18n.Tr("editor.agent.check_again", "Check Again")) {
		p.d.Host.SetCommand(strings.TrimSpace(p.path))
		p.check()
	}
}

// Checked reports whether the last look for Claude Code has finished.
func (p *Panel) Checked() bool {
	return !p.checking && (p.ver != "" || p.verErr != nil)
}

// Busy reports whether the model is answering.
func (p *Panel) Busy() bool {
	return p.d.Host.Busy()
}

// UndoCard takes back an applied card while it is the last step, as its Undo button does.
func (p *Panel) UndoCard(id int) error {
	return p.d.Host.Undo(id)
}

// applicable counts the waiting cards Apply All takes: every pending one that is not destructive.
func (p *Panel) applicable() int {
	n := 0
	for i := range p.cards {
		if c := &p.cards[i]; c.State == agentui.Pending && !c.Destructive {
			n++
		}
	}
	return n
}

// ApplyAll applies every waiting card that is not destructive, and those that come until the answer ends.
func (p *Panel) ApplyAll() {
	p.turnAll = true
	for i := range p.cards {
		if c := &p.cards[i]; c.State == agentui.Pending && !c.Destructive {
			p.d.Host.Decide(c.ID, true)
		}
	}
}
