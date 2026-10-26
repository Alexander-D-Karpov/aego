// Package agent is the editor panel that talks to the coding agent and shows its steps, proposals and questions (D343).
package agent

import (
	"path"
	"strconv"
	"strings"
	"time"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/follow"
	"aego/editor/core/services"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/ui/core"
	"aego/ui/richtext"
	"aego/ui/widgets"
	"aego/ui/widgets/palette"
)

const (
	ID   = "agent"
	Icon = "panel.agent"
	// maxRows caps the rows a laid-out result shows before "+N more".
	maxRows = 12
)

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
	// Open follows a link of a step or an answer: doc:<id>, ref:<id>, node:<file>#<id>, asset:<path>, file:<path>:<line>.
	Open func(link string)
	// Now is the clock of the turn separators; nil uses the wall clock.
	Now func() time.Time
	// Follow leads the editor after the agent; Seconds is its clock, the frame time (11b5b).
	Follow  *follow.Service
	Seconds func() float64
	// FollowChanged saves the Follow button's state in the preferences.
	FollowChanged func(on bool)
	// Selection is the user's selection, which a pinned Context sends with each message.
	Selection interface{ Selected() []ids.NodeID }
	// BudgetsChanged saves the read limits in the preferences.
	BudgetsChanged func(b agentui.Budgets)
}

type avail struct {
	version string
	err     error
}

type Panel struct {
	d         Deps
	blocks    []block
	old       []block
	events    []agentui.Event
	cards     []agentui.Card
	questions []agentui.Question
	seen      map[int]bool
	asked     map[int]bool
	answered  map[int]agentui.State
	logged    map[int]agentui.State
	input     string
	prevLen   int
	scroll    float32
	content   float32
	bar       widgets.ScrollState
	// stick keeps the transcript at its end while new output comes; scrolling up turns it off until the end is reached again.
	stick    bool
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
	viewing  string
	pal      *palette.Palette
	mentions []mention
	picked   []mention
	notified bool
	turnAll  bool
	meta     string
	session  string
	fits     [4]fit
	menu     core.ID
	history  core.ID
	timeline []dot
	gap      float32
	undoTo   int
	undoMenu core.ID
	// meter counts the context of the conversation; the Context popup shows its layout (11b5c).
	meter        *agentui.Meter
	layout       agentui.Layout
	layoutRev    uint64
	ctxPop       core.ID
	ctxOpen      [agentui.CatCount]bool
	ctxWant      bool
	ctxLabel     string
	ctxLabelRev  uint64
	itemFits     map[int]*fit
	pinScene     bool
	pinSelection bool
	seed         string
	summarizing  bool
	summary      strings.Builder
}

// mention is an @ reference: the input shows Label, the message sends Full.
type mention struct {
	Label, Full, Icon string
}

func New(d Deps) *Panel {
	return &Panel{d: d, seen: map[int]bool{}, asked: map[int]bool{}, answered: map[int]agentui.State{}, logged: map[int]agentui.State{}, checked: make(chan avail, 1), stick: true,
		menu: core.IDOf("agent.menu"), history: core.IDOf("agent.history"), undoMenu: core.IDOf("agent.undoto"), ctxPop: core.IDOf("agent.context"), itemFits: map[int]*fit{}}
}

func (p *Panel) ID() string {
	return ID
}

func (p *Panel) Title() string {
	if p.waiting() {
		return i18n.Tr("editor.agent.agent", "Agent •")
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
	p.viewing = ""
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

// Questions are the agent's questions waiting for an answer.
func (p *Panel) Questions() []int {
	var out []int
	for i := range p.questions {
		if p.questions[i].State == agentui.Pending {
			out = append(out, p.questions[i].ID)
		}
	}
	return out
}

// Answer answers a waiting question as its buttons do, for tests.
func (p *Panel) Answer(id int, picks []int, text string) {
	p.d.Host.Answer(id, picks, text)
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

func (p *Panel) waiting() bool {
	for i := range p.cards {
		if p.cards[i].State == agentui.Pending {
			return true
		}
	}
	for i := range p.questions {
		if p.questions[i].State == agentui.Pending {
			return true
		}
	}
	return false
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
	now := p.now()
	for _, e := range p.events {
		p.event(e)
		if e.Kind == agentui.EvToolStart && p.d.Follow != nil {
			p.d.Follow.Step(e.Tool, e.Args, now)
		}
	}
	p.cards = host.Cards(p.cards[:0])
	for i := range p.cards {
		c := &p.cards[i]
		if !p.seen[c.ID] {
			p.seen[c.ID] = true
			p.attach(c)
		}
		if c.State == agentui.Pending && p.turnAll && !c.Destructive {
			p.d.Host.Decide(c.ID, true)
			c.State = agentui.Approved
		}
		if st, ok := p.logged[c.ID]; !ok || st != c.State {
			p.logged[c.ID] = c.State
			p.record(agentui.Entry{Kind: "card", Card: c.ID, Text: summary(c), State: c.State.String()})
			p.redraw(c.ID)
		}
	}
	if p.d.Follow != nil {
		p.d.Follow.Cards(p.cards, now)
		p.d.Follow.Tick(now)
	}
	p.questions = host.Questions(p.questions[:0])
	for i := range p.questions {
		q := &p.questions[i]
		if !p.asked[q.ID] {
			p.asked[q.ID] = true
			p.push(block{kind: blockQuestion, question: q.ID, pick: make([]bool, len(q.Options))})
		}
		if st, ok := p.answered[q.ID]; !ok || st != q.State {
			p.answered[q.ID] = q.State
			for j := range p.blocks {
				if b := &p.blocks[j]; b.kind == blockQuestion && b.question == q.ID {
					b.dirty()
				}
			}
			if q.State != agentui.Pending {
				p.record(agentui.Entry{Kind: "question", Card: q.ID, Text: q.Text + "\n" + q.Answer, State: q.State.String()})
			}
		}
	}
	pending := p.waiting()
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

// redraw measures the block of a card again after its state changed; a decided card folds.
func (p *Panel) redraw(card int) {
	c := p.card(card)
	for i := range p.blocks {
		b := &p.blocks[i]
		if b.kind == blockCard && b.card == card {
			b.dirty()
			if c != nil && c.State != agentui.Pending && c.State != agentui.Approved {
				b.open = false
			}
		}
	}
}

func (p *Panel) event(e agentui.Event) {
	p.meterEvent(e)
	switch e.Kind {
	case agentui.EvText:
		p.text(e.Text)
		p.record(agentui.Entry{Kind: "text", Text: e.Text})
	case agentui.EvToolStart:
		p.toolStart(e)
		p.record(agentui.Entry{Kind: "tool", Tool: e.Tool, CallID: e.CallID, Args: e.Args})
	case agentui.EvToolResult:
		p.toolResult(e)
		p.record(agentui.Entry{Kind: "result", Tool: e.Tool, CallID: e.CallID, Text: e.Result, Failed: e.Failed})
	case agentui.EvError:
		p.push(block{kind: blockError, text: e.Text})
		p.record(agentui.Entry{Kind: "error", Text: e.Text})
	case agentui.EvTurnEnd:
		p.meta = turnMeta(e.Cost, e.Millis)
	}
	if e.Kind == agentui.EvTurnEnd || e.Kind == agentui.EvError {
		p.turnAll = false
	}
}

// turnMeta is the small line under the input: what the last answer cost and how long it took.
func turnMeta(cost float64, millis int) string {
	var parts []string
	if cost > 0 {
		parts = append(parts, "$"+strconv.FormatFloat(cost, 'f', 2, 64))
	}
	if millis > 0 {
		parts = append(parts, i18n.Tr("editor.agent.seconds", "{n} s", i18n.Str("n", strconv.FormatFloat(float64(millis)/1000, 'f', 1, 64))))
	}
	return strings.Join(parts, " · ")
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
	full := text
	for _, m := range p.picked {
		full = strings.Replace(full, m.Label, m.Full, 1)
	}
	now := time.Now()
	if p.d.Now != nil {
		now = p.d.Now()
	}
	if len(p.blocks) > 0 {
		p.push(block{kind: blockTurn, at: now})
	}
	if p.session == "" {
		p.session = firstLine(text)
	}
	p.push(block{kind: blockUser, text: text, at: now})
	p.record(agentui.Entry{Kind: "user", Text: full})
	p.input, p.prevLen, p.stick, p.picked = "", 0, true, p.picked[:0]
	if err := p.d.Host.Send(p.outgoing(full)); err != nil {
		p.push(block{kind: blockError, text: err.Error()})
		p.record(agentui.Entry{Kind: "error", Text: err.Error()})
	}
}

// sendText sends text from a button: an example, Retry or Ask Agent to Fix; while the agent works it waits in the input.
func (p *Panel) sendText(text string) {
	p.input = text
	if !p.d.Host.Busy() {
		p.send()
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

// NewSession ends the conversation and starts an empty transcript; the next message opens a new session log.
func (p *Panel) NewSession() {
	p.d.Host.Reset()
	if p.meter != nil {
		p.meter.Reset()
	}
	p.seed, p.summarizing = "", false
	if p.log != nil {
		p.log.Close()
		p.log = nil
	}
	p.blocks, p.old, p.viewing, p.session, p.meta = nil, nil, "", "", ""
	p.past, p.scroll, p.stick = nil, 0, true
}

func (p *Panel) Draw(f *services.Frame, r mathx.Rect) {
	ui := f.UI
	th := &ui.Theme
	if p.verErr != nil && !p.d.Host.Running() {
		p.drawMissing(ui)
		return
	}
	bar := mathx.Rect{X: r.X, Y: r.Y, W: r.W, H: th.RowHeight + th.Padding*2}
	p.header(ui, bar)
	if p.showWhere() {
		wr := mathx.Rect{X: r.X, Y: bar.Y + bar.H, W: r.W, H: th.RowHeight + 4}
		p.where(ui, wr)
		bar.H += wr.H
	}
	inputH := th.RowHeight + 2*th.Padding + 12
	if len(p.picked) > 0 {
		inputH += th.RowHeight + th.Spacing
	}
	metaH := th.RowHeight * 0.85
	top := bar.Y + bar.H
	body := mathx.Rect{X: r.X, Y: top, W: max(0, r.W-th.ScrollbarW-2), H: max(0, r.Y+r.H-top-inputH-metaH)}
	ui.Rect(mathx.Rect{X: r.X, Y: top - 1, W: r.W, H: 1}, th.Border)
	p.transcript(ui, body, mathx.Rect{X: body.X + body.W + 2, Y: body.Y, W: th.ScrollbarW, H: body.H})
	p.activity(ui, mathx.Rect{X: r.X + th.Padding, Y: body.Y + body.H + 2, W: max(0, r.W-2*th.Padding), H: 10})
	p.footer(ui, mathx.Rect{X: r.X + th.Padding, Y: body.Y + body.H + th.Padding + 12, W: max(0, r.W-2*th.Padding), H: inputH - th.Padding - 12 + metaH})
	p.drawMentions(ui)
}

// status is what the agent does now, as the header says it.
func (p *Panel) status(ui *core.Context) (string, mathx.Color) {
	th := &ui.Theme
	switch {
	case p.waiting():
		return i18n.Tr("editor.agent.status_waiting", "waiting for you"), th.Accent
	case p.d.Host.Busy() && p.running() != nil:
		return i18n.Tr("editor.agent.status_tool", "working"), th.TextDim
	case p.d.Host.Busy():
		return i18n.Tr("editor.agent.status_thinking", "thinking"), th.TextDim
	}
	return i18n.Tr("editor.agent.status_idle", "idle"), th.TextDisabled
}

// running is the call the agent waits on, nil when none.
func (p *Panel) running() *step {
	for i := len(p.blocks) - 1; i >= 0 && i >= len(p.blocks)-4; i-- {
		b := &p.blocks[i]
		for j := len(b.steps) - 1; j >= 0; j-- {
			if !b.steps[j].done {
				return &b.steps[j]
			}
		}
	}
	return nil
}

func (p *Panel) header(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	host := p.d.Host
	ui.Rect(r, th.Header)
	r = mathx.Rect{X: r.X + th.Padding, Y: r.Y + th.Padding, W: r.W - 2*th.Padding, H: th.RowHeight}
	widgets.IconAt(ui, mathx.Rect{X: r.X, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, "agent.avatar", th.Agent.Presence)
	x := r.X + 22
	right := r.X + r.W
	mw := r.H
	if widgets.IconButton(ui, ui.ID("more"), mathx.Rect{X: right - mw, Y: r.Y, W: mw, H: r.H}, "ui.more", i18n.Tr("editor.agent.menu", "Session")) {
		ui.OpenPopup(p.menu, mathx.Vec2{X: max(r.X, right-220), Y: r.Y + r.H})
	}
	right -= mw + 6
	if f := p.d.Follow; f != nil {
		fr := mathx.Rect{X: right - r.H, Y: r.Y, W: r.H, H: r.H}
		if f.On() {
			ui.RoundRect(fr, th.Radius.Button, th.ButtonActive)
		}
		tip := i18n.Tr("editor.agent.follow_tip", "Follow the agent: the editor shows where it works")
		if widgets.IconButton(ui, ui.ID("follow"), fr, "panel.watch", tip) {
			f.SetOn(!f.On())
			if p.d.FollowChanged != nil {
				p.d.FollowChanged(f.On())
			}
		}
		right -= r.H + 6
	}
	right = p.contextButton(ui, right, r)
	auto := host.AutoApply()
	alabel := i18n.Tr("editor.agent.auto_apply", "Auto-apply")
	aw := ui.TextSize(alabel).X + 20
	if widgets.ToggleButton(ui, ui.ID("auto"), mathx.Rect{X: right - aw, Y: r.Y, W: aw, H: r.H}, alabel, &auto) {
		host.SetAutoApply(auto)
	}
	widgets.Tooltip(ui, ui.ID("auto"), i18n.Tr("editor.agent.apply_changes_without_asking_removals", "Apply changes without asking; removals, moves and deletes still wait for you"))
	right -= aw + 8
	status, col := p.status(ui)
	sw := ui.TextSize(status).X + 10
	if host.Busy() {
		sw += 10
	}
	name := p.session
	if p.viewing != "" {
		name = sessionName(p.d.Root, p.viewing)
	}
	if name == "" {
		name = i18n.Tr("editor.agent.new_session", "New session")
	}
	if right-x-sw-10 < 40 {
		status, sw = "", 10
	}
	nw := max(0, right-x-sw-10)
	shown := p.fits[0].get(ui, name, nw)
	nr := mathx.Rect{X: x, Y: r.Y, W: ui.TextSize(shown).X + 4, H: r.H}
	id := ui.ID("session")
	if ui.Clicked(id, nr) {
		p.past = nil
		ui.OpenPopup(p.history, mathx.Vec2{X: nr.X, Y: r.Y + r.H})
	}
	tc := th.Text
	if ui.Hot() == id {
		tc = th.Accent
	}
	ui.Text(shown, mathx.Vec2{X: x, Y: ui.TextY(r)}, tc)
	widgets.Tooltip(ui, id, i18n.Tr("editor.agent.history_tip", "Past sessions"))
	sx := x + nr.W + 8
	if host.Busy() {
		pulse := 0.55 + 0.45*mathx.Sin(float32(ui.Time)*5)
		dot := col
		dot.A *= pulse
		ui.RoundRect(mathx.Rect{X: sx, Y: r.Y + r.H/2 - 3, W: 6, H: 6}, 3, dot)
		sx += 10
	}
	if status != "" {
		ui.Text(status, mathx.Vec2{X: sx, Y: ui.TextY(r)}, col)
	}
	p.sessionMenu(ui)
	p.historyMenu(ui)
	p.contextPopup(ui)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func (p *Panel) sessionMenu(ui *core.Context) {
	if !widgets.ContextMenu(ui, p.menu) {
		return
	}
	host := p.d.Host
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.agent.menu_new", "New Session"), "", "agent.new", true, false) {
		p.NewSession()
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.agent.menu_history", "History…"), "", "panel.history", true, false) {
		p.past = nil
		at := ui.Mouse()
		ui.CloseAllPopups()
		ui.OpenPopup(p.history, at)
	}
	widgets.Separator(ui)
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.agent.menu_plan", "Plan Only"), "", "agent.plan", true, host.PlanOnly()) {
		host.SetPlanOnly(!host.PlanOnly())
	}
	if widgets.MenuItemIcon(ui, i18n.Tr("editor.agent.stop", "Stop"), "", "play.stop", host.Busy(), false) {
		host.Cancel()
	}
	widgets.EndContextMenu(ui)
}

func (p *Panel) historyMenu(ui *core.Context) {
	if !widgets.ContextMenu(ui, p.history) {
		return
	}
	if p.past == nil {
		p.past, _ = agentui.List(p.d.Root)
		if p.past == nil {
			p.past = []string{}
		}
		p.names = p.names[:0]
		for _, id := range p.past {
			p.names = append(p.names, sessionName(p.d.Root, id))
		}
	}
	if widgets.MenuItem(ui, i18n.Tr("editor.agent.current_session", "Current session"), "", true, p.viewing == "") {
		p.viewing, p.old, p.stick = "", nil, true
	}
	for i, id := range p.past {
		if p.log != nil && id == p.log.ID {
			continue
		}
		if widgets.MenuItem(ui, p.names[i], "", true, p.viewing == id) {
			p.open(id)
		}
	}
	widgets.EndContextMenu(ui)
}

// sessionName is a past session by its first message.
func sessionName(root, id string) string {
	entries, err := agentui.Load(root, id)
	if err == nil {
		for _, e := range entries {
			if e.Kind == "user" && e.Text != "" {
				line := firstLine(e.Text)
				if r := []rune(line); len(r) > 48 {
					line = string(r[:47]) + "…"
				}
				return line
			}
		}
	}
	return id
}

func (p *Panel) open(id string) {
	entries, err := agentui.Load(p.d.Root, id)
	if err != nil {
		p.d.Notices.Notify(context.LevelError, err.Error())
		return
	}
	p.viewing, p.old, p.scroll, p.stick = id, p.blocksFrom(entries), 0, false
}

// blocksFrom rebuilds a past session: steps get their looks again from the saved arguments and results.
func (p *Panel) blocksFrom(entries []agentui.Entry) []block {
	saved := p.blocks
	p.blocks = nil
	for _, e := range entries {
		switch e.Kind {
		case "user":
			if len(p.blocks) > 0 {
				p.push(block{kind: blockTurn, at: e.Time})
			}
			p.push(block{kind: blockUser, text: e.Text, at: e.Time})
		case "text":
			p.text(e.Text)
		case "tool":
			p.toolStart(agentui.Event{Tool: e.Tool, CallID: e.CallID, Args: e.Args})
		case "result":
			p.toolResult(agentui.Event{Tool: e.Tool, CallID: e.CallID, Result: e.Text, Failed: e.Failed})
		case "card":
			if b := p.pastCard(e.Card); b != nil {
				b.text = e.Text
				continue
			}
			for i := len(p.blocks) - 1; i >= 0; i-- {
				if b := &p.blocks[i]; b.kind == blockCard && b.card == 0 {
					b.card, b.text, b.open = -e.Card, e.Text, false
					break
				}
			}
		case "error":
			p.push(block{kind: blockError, text: e.Text})
		case "notice":
			p.push(block{kind: blockNotice, text: e.Text})
		}
	}
	out := p.blocks
	p.blocks = saved
	return out
}

func (p *Panel) pastCard(id int) *block {
	for i := range p.blocks {
		if p.blocks[i].kind == blockCard && p.blocks[i].card == -id {
			return &p.blocks[i]
		}
	}
	return nil
}

func (p *Panel) footer(ui *core.Context, r mathx.Rect) {
	th := &ui.Theme
	host := p.d.Host
	y := r.Y
	if len(p.picked) > 0 {
		x := r.X
		for i := range p.picked {
			m := &p.picked[i]
			w := ui.TextSize(m.Label).X + 30
			if x+w > r.X+r.W {
				break
			}
			chip := mathx.Rect{X: x, Y: y, W: w, H: th.RowHeight}
			ui.RoundRect(chip, th.Radius.Button, th.Agent.User)
			widgets.IconAt(ui, mathx.Rect{X: x + 5, Y: y + (chip.H-14)/2, W: 14, H: 14}, m.Icon, th.TextDim)
			ui.Text(m.Label, mathx.Vec2{X: x + 23, Y: ui.TextY(chip)}, th.Text)
			x += w + 4
		}
		y += th.RowHeight + th.Spacing
	}
	row := mathx.Rect{X: r.X, Y: y, W: r.W, H: th.RowHeight}
	locked := p.viewing != ""
	busy := host.Busy()
	label := i18n.Tr("editor.agent.send", "Send")
	if busy {
		label = i18n.Tr("editor.agent.stop", "Stop")
	}
	bw := ui.TextSize(label).X + 24
	id := ui.ID("message")
	if p.focus {
		ui.SetFocus(id)
		p.focus = false
	}
	fr := mathx.Rect{X: row.X, Y: row.Y, W: max(0, row.W-bw-6), H: row.H}
	enter := ui.Focus() == id && (ui.KeyPressed(input.KeyEnter) || ui.KeyPressed(input.KeyKPEnter))
	widgets.TextFieldRect(ui, id, fr, &p.input, true)
	if p.input == "" && ui.Focus() != id {
		hint := i18n.Tr("editor.agent.message_the_agent_to_mention", "Message the agent, @ to mention")
		if locked {
			hint = i18n.Tr("editor.agent.past_read_only", "A past session is read-only; pick Current session in the history to write")
		}
		ui.Text(p.fits[1].get(ui, hint, fr.W-8), mathx.Vec2{X: fr.X + 4, Y: ui.TextY(fr)}, th.TextDisabled)
	}
	if len(p.input) > p.prevLen && strings.HasSuffix(p.input, "@") {
		p.input = strings.TrimSuffix(p.input, "@")
		p.openMentions(ui)
	}
	p.prevLen = len(p.input)
	p.dropMentions()
	br := mathx.Rect{X: row.X + row.W - bw, Y: row.Y, W: bw, H: row.H}
	if busy {
		if widgets.ButtonRect(ui, ui.ID("stop"), br, label) {
			host.Cancel()
		}
	} else if sent := primary(ui, ui.ID("send"), br, label, !locked && strings.TrimSpace(p.input) != ""); sent || (enter && !locked) {
		p.send()
		p.focus = true
	}
	meta := p.meta
	if host.PlanOnly() {
		meta = i18n.Tr("editor.agent.plan_on", "Plan only: the agent describes changes and makes none")
	}
	if meta != "" {
		mr := mathx.Rect{X: r.X, Y: row.Y + row.H + 2, W: r.W, H: th.RowHeight * 0.8}
		ui.Text(p.fits[2].get(ui, meta, mr.W), mathx.Vec2{X: mr.X, Y: ui.TextY(mr)}, th.TextDisabled)
	}
}

// primary is the accent button of a row: Send, Apply.
func primary(ui *core.Context, id core.ID, r mathx.Rect, label string, enabled bool) bool {
	th := &ui.Theme
	if !enabled {
		ui.RoundRect(r, th.Radius.Button, th.Widget)
		ui.TextIn(r, label, th.TextDisabled, core.AlignCenter)
		return false
	}
	hit := ui.Clicked(id, r)
	fill := th.Accent
	if ui.Hot() == id {
		fill = fill.Lerp(th.Text, 0.15)
	}
	ui.RoundRect(r, th.Radius.Button, fill)
	ui.TextIn(r, label, th.Background, core.AlignCenter)
	return hit
}

func (p *Panel) openMentions(ui *core.Context) {
	if p.pal == nil {
		p.pal = palette.New(core.IDOf("agent.mentions"))
		p.pal.Title = i18n.Tr("editor.agent.mention", "Mention")
		p.pal.Hint = i18n.Tr("editor.agent.mention_hint", "Enter insert · Esc close")
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
			items = append(items, palette.Item{Key: "node:" + id.String(), Label: name, Category: i18n.Tr("editor.agent.mention_nodes", "Nodes"), Detail: path.Base(d.Path), Icon: "cat.node"})
			p.mentions = append(p.mentions, mention{Label: "@" + name, Full: "@" + name + " (#" + id.String() + " in " + d.Path + ")", Icon: "cat.node"})
		}
	}
	if p.d.Assets != nil {
		for _, a := range p.d.Assets() {
			icon := assetIcon(a)
			items = append(items, palette.Item{Key: "asset:" + a, Label: path.Base(a), Category: i18n.Tr("editor.agent.mention_assets", "Assets"), Detail: a, Icon: icon})
			p.mentions = append(p.mentions, mention{Label: "@" + path.Base(a), Full: "@" + a, Icon: icon})
		}
	}
	p.pal.SetItems(items)
	p.pal.Show(ui)
}

func assetIcon(a string) string {
	switch path.Ext(a) {
	case ".aes":
		return "file.script"
	case ".aescene":
		return "file.scene"
	case ".aeprefab":
		return "file.prefab"
	case ".png", ".jpg":
		return "file.texture"
	case ".wav", ".ogg":
		return "file.audio"
	}
	return "file.unknown"
}

// dropMentions forgets the chips whose label the user deleted from the input.
func (p *Panel) dropMentions() {
	n := 0
	for _, m := range p.picked {
		if strings.Contains(p.input, m.Label) {
			p.picked[n] = m
			n++
		}
	}
	p.picked = p.picked[:n]
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
		m := p.mentions[res.Pick]
		if p.input != "" && !strings.HasSuffix(p.input, " ") {
			p.input += " "
		}
		p.input += m.Label + " "
		p.picked = append(p.picked, m)
		p.prevLen = len(p.input)
		p.pal.Hide(ui)
		p.focus = true
	}
}

func (p *Panel) drawMissing(ui *core.Context) {
	widgets.Label(ui, i18n.Tr("editor.agent.claude_code_is_not_available", "Claude Code is not available"))
	widgets.LabelWrapped(ui, i18n.Tr("editor.agent.install_help", "The agent runs Claude Code, the claude command. Install it, sign in once by running claude in a terminal, then check again. If it is installed where the editor does not look, set its path below."), true)
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

func (p *Panel) card(id int) *agentui.Card {
	for i := range p.cards {
		if p.cards[i].ID == id {
			return &p.cards[i]
		}
	}
	return nil
}

func (p *Panel) question(id int) *agentui.Question {
	for i := range p.questions {
		if p.questions[i].ID == id {
			return &p.questions[i]
		}
	}
	return nil
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

// Expand opens every row and step of the transcript, for screenshots and tests.
func (p *Panel) Expand() {
	for i := range p.blocks {
		b := &p.blocks[i]
		b.open = true
		for j := range b.steps {
			b.steps[j].open = b.steps[j].look.Detail.Kind != agentui.DetailNone
		}
		b.dirty()
	}
}

// showWhere reports whether the row under the header shows: while following, or while the agent's marker shows.
func (p *Panel) showWhere() bool {
	f := p.d.Follow
	if f == nil {
		return false
	}
	if f.On() {
		return true
	}
	_, _, ok := f.Model.Where(follow.Agent, p.now())
	return ok
}
