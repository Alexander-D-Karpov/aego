package agent

import (
	"strconv"
	"strings"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/engine/i18n"
	"aego/ui/core"
	"aego/ui/widgets"
)

// warnShare is how full the context gets before the Context button turns yellow.
const warnShare = 0.8

// summaryPrompt asks the agent for the summary a new session starts from.
const summaryPrompt = "Write a summary of this session for a new session that continues it: what the user asked for, what you changed (files and node names), what you decided and why, and what is left to do. Do not call tools; answer with the summary only."

// order is how the popup lists the categories: the conversation first, the fixed parts after.
var order = [...]agentui.Category{agentui.CatMessages, agentui.CatScene, agentui.CatCode, agentui.CatDocs, agentui.CatProject, agentui.CatResults,
	agentui.CatInstructions, agentui.CatTools, agentui.CatSystem}

func catName(c agentui.Category) string {
	switch c {
	case agentui.CatSystem:
		return i18n.Tr("editor.agent.ctx.system", "System prompt")
	case agentui.CatTools:
		return i18n.Tr("editor.agent.ctx.tools", "Tool definitions")
	case agentui.CatInstructions:
		return i18n.Tr("editor.agent.ctx.instructions", "Instructions")
	case agentui.CatScene:
		return i18n.Tr("editor.agent.ctx.scene", "Scene")
	case agentui.CatCode:
		return i18n.Tr("editor.agent.ctx.code", "Code")
	case agentui.CatDocs:
		return i18n.Tr("editor.agent.ctx.docs", "Docs")
	case agentui.CatProject:
		return i18n.Tr("editor.agent.ctx.project", "Project & schema")
	case agentui.CatResults:
		return i18n.Tr("editor.agent.ctx.results", "Tool results")
	}
	return i18n.Tr("editor.agent.ctx.messages", "Messages")
}

func catColor(th *core.Theme, c agentui.Category) mathx.Color {
	switch c {
	case agentui.CatSystem:
		return th.TextDisabled
	case agentui.CatTools:
		return th.Series[7]
	case agentui.CatInstructions:
		return th.Series[3]
	case agentui.CatScene:
		return th.Series[1]
	case agentui.CatCode:
		return th.Series[2]
	case agentui.CatDocs:
		return th.Series[4]
	case agentui.CatProject:
		return th.Series[6]
	case agentui.CatResults:
		return th.Series[5]
	}
	return th.Series[0]
}

// tokens is a token count as the popup writes it: 940, 22.6k, 1M.
func tokens(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1_000_000:
		return strings.TrimSuffix(strconv.FormatFloat(float64(n)/1000, 'f', 1, 64), ".0") + "k"
	}
	s := strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + "M"
}

func percent(part, whole int) string {
	if whole <= 0 {
		return "0 %"
	}
	return strconv.FormatFloat(100*float64(part)/float64(whole), 'f', 1, 64) + " %"
}

// meterOf is the meter of the conversation, made on the first message when the host knows what the model reads first.
func (p *Panel) meterOf() *agentui.Meter {
	if p.meter == nil {
		instr, tools := p.d.Host.ContextBase()
		p.meter = agentui.NewMeter(instr, tools)
	}
	return p.meter
}

// layout lays the meter out again when it changed.
func (p *Panel) contextLayout() *agentui.Layout {
	if p.meter != nil && p.meter.Rev() != p.layoutRev {
		p.layoutRev = p.meter.Rev()
		p.meter.Layout(&p.layout)
	}
	return &p.layout
}

// meterEvent keeps the meter with the conversation: messages, calls, results and the CLI's usage.
func (p *Panel) meterEvent(e agentui.Event) {
	m := p.meter
	if m == nil {
		return
	}
	switch e.Kind {
	case agentui.EvText:
		if e.Delta {
			m.TextDelta(e.MessageID, e.Text)
		} else {
			m.Text(e.Text)
		}
		if p.summarizing {
			p.summary.WriteString(e.Text)
		}
	case agentui.EvToolStart:
		m.Call(e.Tool, e.CallID, e.Args, p.d.Host.Describe(e.Tool, e.Args, "", false).Title)
	case agentui.EvToolResult:
		st := p.d.Host.Describe(e.Tool, p.argsOf(e.CallID), e.Result, e.Failed)
		m.Result(e.Tool, e.CallID, e.Result, st.Summary, e.Failed)
	case agentui.EvUsage:
		if !m.Based() {
			if instr, tools := p.d.Host.ContextBase(); instr != "" || tools != "" {
				m.SetBase(instr, tools)
			}
		}
		m.SetModel(e.Model)
		m.SetWindow(e.Window)
		m.Usage(e.Tokens)
	case agentui.EvTurnEnd:
		m.SetWindow(e.Window)
		if p.summarizing {
			p.finishSummary()
		}
	case agentui.EvCompact:
		window := max(m.Window, 1)
		m.Compacted()
		if e.Tokens > 0 {
			p.notice(i18n.Tr("editor.agent.ctx.compacted", "Context compacted at {n}", i18n.Str("n", percent(e.Tokens, window))))
		} else {
			p.notice(i18n.Tr("editor.agent.ctx.compacted_unknown", "Context compacted"))
		}
	case agentui.EvError:
		p.summarizing = false
	}
}

// argsOf is the arguments of a call of this turn, for laying out its result.
func (p *Panel) argsOf(callID string) string {
	for i := len(p.blocks) - 1; i >= 0; i-- {
		for j := range p.blocks[i].steps {
			if s := &p.blocks[i].steps[j]; s.call == callID {
				return s.args
			}
		}
	}
	return ""
}

// outgoing is the message as it goes to the model: a rebuilt history or a summary first, then the pinned editor state.
func (p *Panel) outgoing(text string) string {
	m := p.meterOf()
	switch n, t := m.Dropped(); {
	case p.seed != "":
		p.d.Host.Reset()
		m.Reset()
		m.Rebuilt(p.seed)
		text = p.seed + "\n\n" + text
		p.seed = ""
	case n > 0:
		proj := m.Projection()
		p.d.Host.Reset()
		m.Rebuilt(proj)
		text = proj + "\n\n" + text
		p.notice(i18n.Tr("editor.agent.ctx.rebuilt", "Started the session again without {n} dropped results ({t} tokens fewer)",
			i18n.Str("n", strconv.Itoa(n)), i18n.Str("t", tokens(t))))
	}
	m.User(text)
	if state := p.editorState(); state != "" {
		m.Attach(i18n.Tr("editor.agent.ctx.pinned_state", "Open scene and selection"), state)
		text = state + "\n\n" + text
	}
	return text
}

// editorState is the pinned part of the editor the message carries: the open scene and the selection.
func (p *Panel) editorState() string {
	if !p.pinScene && !p.pinSelection || p.d.Docs == nil {
		return ""
	}
	d := p.d.Docs.Doc()
	if d == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("[Editor] ")
	if p.pinScene {
		b.WriteString("Open scene: " + d.Path + ".")
	}
	if p.pinSelection && p.d.Selection != nil {
		s := d.Scene()
		var names []string
		for _, id := range p.d.Selection.Selected() {
			if h := s.Handle(id); !h.IsZero() && s.IsAlive(h) {
				names = append(names, s.Name(h)+" (#"+id.String()+")")
			}
			if len(names) == 20 {
				break
			}
		}
		if len(names) > 0 {
			b.WriteString(" Selected: " + strings.Join(names, ", ") + ".")
		}
	}
	return b.String()
}

// SummarizeSession asks the agent for a summary and starts a new session from it once the answer is in.
func (p *Panel) SummarizeSession() {
	if p.d.Host.Busy() || p.meter == nil || len(p.queue) > 0 {
		return
	}
	p.summarizing = true
	p.summary.Reset()
	p.notice(i18n.Tr("editor.agent.ctx.summarizing", "Writing a summary for a new session…"))
	p.meter.User(summaryPrompt)
	if err := p.d.Host.Send(summaryPrompt); err != nil {
		p.summarizing = false
		p.push(block{kind: blockError, text: err.Error()})
	}
}

// finishSummary starts the new session: the summary and the nodes the session changed go with the next message.
func (p *Panel) finishSummary() {
	p.summarizing = false
	text := strings.TrimSpace(p.summary.String())
	if text == "" {
		return
	}
	if len(p.queue) > 0 {
		p.notice(i18n.Tr("editor.agent.queue_clear_first", "Send or clear queued messages before starting a new session."))
		return
	}
	var nodes []string
	seen := map[string]bool{}
	for _, c := range p.cards {
		if c.State != agentui.Applied {
			continue
		}
		for _, ch := range c.Changes {
			n := strings.TrimPrefix(ch.Node, "Root/")
			if n != "" && !seen[n] {
				seen[n] = true
				nodes = append(nodes, n)
			}
		}
	}
	seed := "Summary of the previous session in the Aego editor:\n" + text
	if len(nodes) > 0 {
		seed += "\nNodes changed in that session: " + strings.Join(nodes, ", ") + "."
	}
	p.NewSession()
	p.seed = seed
	p.notice(i18n.Tr("editor.agent.ctx.new_from_summary", "New session from the summary; it goes with your next message"))
	p.text(text)
}

// notice adds a line about the session to the transcript and its log.
func (p *Panel) notice(text string) {
	p.push(block{kind: blockNotice, text: text})
	p.record(agentui.Entry{Kind: "notice", Text: text})
}

// ShowMeter shows a meter that did not come from a conversation, for the documentation's screenshots.
func (p *Panel) ShowMeter(m *agentui.Meter) {
	p.meter, p.layoutRev = m, m.Rev()-1
}

// OpenContext opens the Context popup on the next frame with the given categories open, as a click on the button does.
func (p *Panel) OpenContext(open ...agentui.Category) {
	p.ctxWant = true
	for _, c := range open {
		p.ctxOpen[c] = true
	}
}

// PinEditor sends the open scene and the selection with every message, as the popup's check boxes do.
func (p *Panel) PinEditor(scene, selection bool) {
	p.pinScene, p.pinSelection = scene, selection
}

// Seed is the summary the next message of a new session carries, for tests.
func (p *Panel) Seed() string {
	return p.seed
}

// Meter is the conversation's context meter, nil before the first message.
func (p *Panel) Meter() *agentui.Meter {
	return p.meter
}

// contextButton is the header's Context button: a gauge and how full the window is, from the first answer on; yellow past 80 %.
func (p *Panel) contextButton(ui *core.Context, right float32, r mathx.Rect) float32 {
	if p.meter == nil {
		return right
	}
	th := &ui.Theme
	l := p.contextLayout()
	if !l.Measured {
		return right
	}
	if p.ctxLabelRev != p.layoutRev || p.ctxLabel == "" {
		p.ctxLabelRev = p.layoutRev
		p.ctxLabel = strconv.Itoa(int(l.Fraction()*100+0.5)) + "%"
	}
	label := p.ctxLabel
	w := ui.TextSize(label).X + 30
	br := mathx.Rect{X: right - w, Y: r.Y, W: w, H: r.H}
	id := ui.ID("context")
	warn := l.Fraction() >= warnShare || l.NearCompaction()
	bg := th.Widget
	if ui.Hot() == id {
		bg = th.WidgetHot
	}
	if warn {
		bg = th.Warn
		bg.A = 0.25
	}
	ui.RoundRect(br, th.Radius.Button, bg)
	col := th.TextDim
	if warn {
		col = th.Warn
	}
	widgets.IconAt(ui, mathx.Rect{X: br.X + 6, Y: br.Y + (br.H-14)/2, W: 14, H: 14}, "agent.context", col)
	ui.Text(label, mathx.Vec2{X: br.X + 24, Y: ui.TextY(br)}, col)
	if p.ctxWant {
		p.ctxWant = false
		ui.OpenPopup(p.ctxPop, mathx.Vec2{X: max(0, br.X+br.W-360), Y: r.Y + r.H + 4})
	} else if ui.Clicked(id, br) {
		if ui.PopupOpen(p.ctxPop) {
			ui.ClosePopup(p.ctxPop)
		} else {
			ui.OpenPopup(p.ctxPop, mathx.Vec2{X: max(0, br.X+br.W-360), Y: r.Y + r.H + 4})
		}
	}
	if ui.Hot() == id {
		widgets.Tooltip(ui, id, i18n.Tr("editor.agent.ctx.tip", "Context: {used} of {window} tokens", i18n.Str("used", tokens(l.Used)), i18n.Str("window", tokens(l.Window))))
	}
	return right - w - 6
}

// contextPopup lays the context out: a bar over the window, a row per category, its contributions with Drop and Pin.
func (p *Panel) contextPopup(ui *core.Context) {
	if p.meter == nil || !ui.BeginPopup(p.ctxPop, 360) {
		return
	}
	th := &ui.Theme
	l := p.contextLayout()
	ui.PushID("ctx")
	cur := ui.Cursor()
	row := cur.Next(0)
	widgets.IconAt(ui, mathx.Rect{X: row.X, Y: row.Y + (row.H-14)/2, W: 14, H: 14}, "agent.context", th.TextDim)
	ui.Text(i18n.Tr("editor.agent.ctx.title", "Context"), mathx.Vec2{X: row.X + 20, Y: ui.TextY(row)}, th.Text)
	if l.Model != "" {
		ui.TextIn(row, l.Model, th.TextDisabled, core.AlignRight)
	}
	row = cur.Next(0)
	used := i18n.Tr("editor.agent.ctx.used", "Used: {n}", i18n.Str("n", percent(l.Used, l.Window)))
	if !l.Measured {
		used = i18n.Tr("editor.agent.ctx.estimated", "About {n}, until the first answer", i18n.Str("n", percent(l.Used, l.Window)))
	}
	ui.Text(used, mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.Text)
	ui.TextIn(row, tokens(l.Used)+" / "+tokens(l.Window), th.TextDim, core.AlignRight)
	p.contextBar(ui, cur.Next(10), l)
	cur.Next(4)
	for _, c := range order {
		share := &l.Rows[c]
		if share.Tokens == 0 && c != agentui.CatMessages {
			continue
		}
		p.categoryRow(ui, cur.Next(0), c, share, l)
		if p.ctxOpen[c] {
			for _, i := range share.Items {
				p.itemRow(ui, cur.Next(0), &p.meter.Items()[i])
			}
		}
	}
	p.plainRow(ui, cur.Next(0), th.Border, i18n.Tr("editor.agent.ctx.buffer", "Compaction buffer"), l.Buffer, l.Window)
	p.plainRow(ui, cur.Next(0), th.Widget, i18n.Tr("editor.agent.ctx.free", "Free"), l.Free, l.Window)
	if n, t := p.meter.Dropped(); n > 0 {
		row = cur.Next(0)
		ui.Text(p.fits[3].get(ui, i18n.Tr("editor.agent.ctx.will_drop", "The next message starts the session again without {n} results ({t})",
			i18n.Str("n", strconv.Itoa(n)), i18n.Str("t", tokens(t))), row.W), mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	}
	widgets.Separator(ui)
	row = cur.Next(0)
	ui.Text(i18n.Tr("editor.agent.ctx.attach", "Send with every message"), mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.TextDim)
	if widgets.CheckRow(ui, ui.ID("pin_scene"), i18n.Tr("editor.agent.ctx.pin_scene", "The open scene"), p.pinScene) {
		p.pinScene = !p.pinScene
	}
	if widgets.CheckRow(ui, ui.ID("pin_selection"), i18n.Tr("editor.agent.ctx.pin_selection", "The selection"), p.pinSelection) {
		p.pinSelection = !p.pinSelection
	}
	p.budgetRow(ui)
	widgets.Separator(ui)
	if l.Fraction() >= warnShare || l.NearCompaction() {
		row = cur.Next(0)
		ui.Text(p.fits[3].get(ui, i18n.Tr("editor.agent.ctx.full", "Start a new session or drop old tool results"), row.W), mathx.Vec2{X: row.X, Y: ui.TextY(row)}, th.Warn)
	}
	row = cur.Next(0)
	if primary(ui, ui.ID("summary"), row, i18n.Tr("editor.agent.ctx.new_summary", "New Session with Summary"), !p.d.Host.Busy()) {
		ui.ClosePopup(p.ctxPop)
		p.SummarizeSession()
	}
	ui.PopID()
	ui.EndPopup()
}

// contextBar is the window as one bar: each category in its colour, then the compaction buffer, the rest free.
func (p *Panel) contextBar(ui *core.Context, r mathx.Rect, l *agentui.Layout) {
	th := &ui.Theme
	r = mathx.Rect{X: r.X, Y: r.Y + 2, W: r.W, H: 6}
	ui.RoundRect(r, 3, th.Widget)
	x := r.X
	for _, c := range order {
		w := r.W * float32(l.Rows[c].Tokens) / float32(l.Window)
		if w <= 0 {
			continue
		}
		ui.Rect(mathx.Rect{X: x, Y: r.Y, W: min(w, r.X+r.W-x), H: r.H}, catColor(th, c))
		x += w
	}
	bw := r.W * float32(l.Buffer) / float32(l.Window)
	ui.Rect(mathx.Rect{X: r.X + r.W - bw, Y: r.Y, W: bw, H: r.H}, th.Border)
}

// numbers draws a row's tokens and share at its right.
func numbers(ui *core.Context, r mathx.Rect, n, window int, col mathx.Color) {
	pr := mathx.Rect{X: r.X + r.W - 52, Y: r.Y, W: 52, H: r.H}
	tr := mathx.Rect{X: pr.X - 60, Y: r.Y, W: 56, H: r.H}
	ui.TextIn(tr, tokens(n), col, core.AlignRight)
	ui.TextIn(pr, percent(n, window), col, core.AlignRight)
}

func (p *Panel) categoryRow(ui *core.Context, r mathx.Rect, c agentui.Category, share *agentui.Share, l *agentui.Layout) {
	th := &ui.Theme
	id := ui.IDInt(int(c) + 1)
	if len(share.Items) > 0 {
		if ui.Clicked(id, r) {
			p.ctxOpen[c] = !p.ctxOpen[c]
		}
		if ui.Hot() == id {
			ui.RoundRect(r, th.Radius.Menu, th.WidgetHot)
		}
		widgets.Chevron(ui, mathx.Rect{X: r.X, Y: r.Y, W: 14, H: r.H}, p.ctxOpen[c], th.TextDim)
	}
	ui.RoundRect(mathx.Rect{X: r.X + 16, Y: r.Y + (r.H-10)/2, W: 10, H: 10}, 2, catColor(th, c))
	ui.Text(catName(c), mathx.Vec2{X: r.X + 32, Y: ui.TextY(r)}, th.Text)
	numbers(ui, r, share.Tokens, l.Window, th.TextDim)
}

func (p *Panel) plainRow(ui *core.Context, r mathx.Rect, col mathx.Color, name string, n, window int) {
	th := &ui.Theme
	ui.RoundRect(mathx.Rect{X: r.X + 16, Y: r.Y + (r.H-10)/2, W: 10, H: 10}, 2, col)
	ui.Text(name, mathx.Vec2{X: r.X + 32, Y: ui.TextY(r)}, th.TextDim)
	numbers(ui, r, n, window, th.TextDisabled)
}

// itemRow is one contribution: its label and summary, its tokens, Drop and Pin.
func (p *Panel) itemRow(ui *core.Context, r mathx.Rect, it *agentui.Item) {
	th := &ui.Theme
	x := r.X + 32
	bw := r.H
	pr := mathx.Rect{X: r.X + r.W - bw, Y: r.Y, W: bw, H: r.H}
	dr := mathx.Rect{X: pr.X - bw - 2, Y: r.Y, W: bw, H: r.H}
	tr := mathx.Rect{X: dr.X - 54, Y: r.Y, W: 50, H: r.H}
	col := th.Text
	if it.Dropped {
		col = th.TextDisabled
	}
	label := it.Label
	if it.Detail != "" {
		label += " · " + it.Detail
	}
	text := p.itemFit(ui, it.ID, label, tr.X-x-4)
	ui.Text(text, mathx.Vec2{X: x, Y: ui.TextY(r)}, col)
	if it.Dropped {
		ui.Rect(mathx.Rect{X: x, Y: r.Y + r.H/2, W: ui.TextSize(text).X, H: 1}, th.TextDisabled)
	}
	ui.TextIn(tr, tokens(it.Tokens), th.TextDisabled, core.AlignRight)
	if it.Pinned {
		ui.RoundRect(pr, th.Radius.Button, th.ButtonActive)
	}
	if widgets.IconButton(ui, ui.IDInt(1000+it.ID*2), dr, "edit.remove", i18n.Tr("editor.agent.ctx.drop", "Drop: leave it out of the history from the next message")) {
		p.meter.Drop(it.ID, !it.Dropped)
	}
	if widgets.IconButton(ui, ui.IDInt(1001+it.ID*2), pr, "agent.pin", i18n.Tr("editor.agent.ctx.pin", "Pin: keep it in full when the history is shortened")) {
		p.meter.Pin(it.ID, !it.Pinned)
	}
}

// itemFit keeps the shortened labels of the popup's rows, so an open popup does not allocate each frame.
func (p *Panel) itemFit(ui *core.Context, id int, label string, w float32) string {
	f := p.itemFits[id]
	if f == nil {
		f = &fit{}
		p.itemFits[id] = f
	}
	return f.get(ui, label, w)
}

// budgetRow edits the read limits, two to a row: nodes and depth of scene_tree, docs_search results, lines of a script read whole.
func (p *Panel) budgetRow(ui *core.Context) {
	th := &ui.Theme
	cur := ui.Cursor()
	r := cur.Next(0)
	ui.Text(i18n.Tr("editor.agent.ctx.limits", "Read limits"), mathx.Vec2{X: r.X, Y: ui.TextY(r)}, th.TextDim)
	b := p.d.Host.Budgets()
	fields := [...]struct {
		label, tip string
		v          *int
	}{
		{i18n.Tr("editor.agent.ctx.tree_nodes", "Nodes"), i18n.Tr("editor.agent.ctx.tree_nodes_tip", "Nodes of a scene tree the agent reads at once"), &b.TreeNodes},
		{i18n.Tr("editor.agent.ctx.tree_depth", "Depth"), i18n.Tr("editor.agent.ctx.tree_depth_tip", "Levels of a scene tree the agent reads at once"), &b.TreeDepth},
		{i18n.Tr("editor.agent.ctx.doc_results", "Docs"), i18n.Tr("editor.agent.ctx.doc_results_tip", "Results of a documentation search"), &b.DocResults},
		{i18n.Tr("editor.agent.ctx.script_lines", "Lines"), i18n.Tr("editor.agent.ctx.script_lines_tip", "A longer script is read in parts"), &b.ScriptLines},
	}
	changed := false
	for i := range fields {
		f := &fields[i]
		if i%2 == 0 {
			r = cur.Next(0)
		}
		w := (r.W - 6) / 2
		fr := mathx.Rect{X: r.X + float32(i%2)*(w+6), Y: r.Y, W: w, H: r.H}
		v := int64(*f.v)
		id := ui.IDInt(2000 + i)
		if widgets.DragInt64(ui, id, fr, &v, widgets.NumberOpts{Speed: 1, Min: 1, HasMin: true, Lead: ui.TextSize(f.label).X + 8, Unit: ""}).End {
			changed = true
		}
		*f.v = int(v)
		ui.Text(f.label, mathx.Vec2{X: fr.X + 4, Y: ui.TextY(fr)}, th.TextDisabled)
		widgets.Tooltip(ui, id, f.tip)
	}
	if b != p.d.Host.Budgets() {
		p.d.Host.SetBudgets(b)
		if changed && p.d.BudgetsChanged != nil {
			p.d.BudgetsChanged(b)
		}
	}
}
