package agentui

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"aego/engine/i18n"
)

// Category is a kind of content in the model's context window, as the Context popup groups it.
type Category uint8

const (
	// CatSystem is the CLI's own prompt and the user's settings, measured at the first answer of a session.
	CatSystem Category = iota
	// CatTools is the tool definitions the MCP server gives the model.
	CatTools
	// CatInstructions is the editor's guide for the agent.
	CatInstructions
	CatScene
	CatCode
	CatDocs
	CatProject
	CatResults
	CatMessages
	CatCount
)

const (
	// DefaultWindow is the context size until the CLI reports the model's own.
	DefaultWindow = 200_000
	// CompactBuffer is the room the CLI keeps free; history is compacted when the context reaches it.
	CompactBuffer = 33_000
	// proseChars and codeChars are characters per token of prose and of code and JSON; tool results come as JSON with escapes and hex ids.
	proseChars = 3.5
	codeChars  = 1.7
	// minScale and maxScale bound how far the usage can stretch the estimates; past them the rest goes to Messages.
	minScale = 0.6
	maxScale = 2.5
)

// Item is one contribution to the context: a tool result, a turn's messages, the pinned editor state.
type Item struct {
	ID    int
	Cat   Category
	Label string
	// Detail is the result's summary, such as "48 nodes".
	Detail string
	Tool   string
	CallID string
	Turn   int
	Text   string
	// Est is the estimated tokens; Tokens is the estimate calibrated against the CLI's usage.
	Est     int
	Tokens  int
	Dropped bool
	Pinned  bool
	// Failed results are dropped from a rebuilt history as a matter of course.
	Failed bool
}

// Share is one category of the layout.
type Share struct {
	Cat    Category
	Tokens int
	Items  []int
}

// Layout is the context as the popup draws it: used, free and the compaction buffer, by category.
type Layout struct {
	Model  string
	Window int
	Used   int
	Free   int
	Buffer int
	Rows   [CatCount]Share
	// Measured is true once the CLI reported usage; before that the numbers are estimates only.
	Measured bool
}

// Meter keeps the context the agent's conversation uses: the CLI's usage for the total, the editor's estimates for its parts.
type Meter struct {
	Model  string
	Window int
	Used   int
	items  []Item
	base   int
	tools  int
	instr  int
	first  bool
	turn   int
	nextID int
	// open is the Messages item of the turn being answered.
	open int
	// starts holds each running call by id until its result comes.
	starts map[string]call
	rev    uint64
	based  bool
}

// SetBase is what the model reads before the first message, once the host knows it; until then the CLI's prompt includes it.
func (m *Meter) SetBase(instructions, tools string) {
	m.instr, m.tools = Estimate(instructions), Estimate(tools)
	m.based = true
	m.rev++
}

// Based reports whether the meter knows the editor's guide and tool definitions.
func (m *Meter) Based() bool {
	return m.based
}

type call struct {
	tool, label string
}

// Rev changes whenever the meter does, so the popup lays out again only then.
func (m *Meter) Rev() uint64 {
	return m.rev
}

// SetWindow is the model's context size as the CLI reports it.
func (m *Meter) SetWindow(n int) {
	if n > 0 && n != m.Window {
		m.Window = n
		m.rev++
	}
}

func (m *Meter) SetModel(name string) {
	if name != "" && name != m.Model {
		m.Model = name
		m.rev++
	}
}

func NewMeter(instructions, tools string) *Meter {
	m := &Meter{Window: DefaultWindow, first: true, starts: map[string]call{}, open: -1}
	m.instr, m.tools = Estimate(instructions), Estimate(tools)
	m.based = instructions != "" || tools != ""
	return m
}

// Estimate is the tokens of text: prose at 3.5 characters a token, code and JSON at 1.7, other scripts by rune.
func Estimate(text string) int {
	if text == "" {
		return 0
	}
	ascii, other, marks := 0, 0, 0
	for _, r := range text {
		switch {
		case r >= utf8.RuneSelf:
			other++
		case strings.ContainsRune(`{}[]":=,;()<>`, r):
			marks++
			ascii++
		default:
			ascii++
		}
	}
	per := proseChars
	if ascii > 0 && float64(marks)/float64(ascii) > 0.06 {
		per = codeChars
	}
	return int(float64(ascii)/per+float64(other)/1.8) + 1
}

// CategoryOf is where a tool's results go.
func CategoryOf(tool string) Category {
	group, verb, _ := strings.Cut(tool, "_")
	switch {
	case group == "script":
		return CatCode
	case group == "docs":
		return CatDocs
	case tool == "ask_user":
		return CatMessages
	case group == "project" && verb == "get", group == "schema", group == "asset", group == "refs", strings.HasPrefix(tool, "data_"):
		return CatProject
	case group == "scene" && (verb == "tree" || verb == "get"), group == "prefab" && verb == "overrides", verb == "get":
		return CatScene
	}
	return CatResults
}

// Reset starts a new session: its first answer measures the CLI's own prompt again.
func (m *Meter) Reset() {
	m.rev++
	m.items, m.Used, m.first, m.turn, m.open = m.items[:0], 0, true, 0, -1
	clear(m.starts)
}

func (m *Meter) add(it Item) *Item {
	m.rev++
	m.nextID++
	it.ID, it.Turn = m.nextID, m.turn
	if it.Est == 0 {
		it.Est = Estimate(it.Text)
	}
	m.items = append(m.items, it)
	return &m.items[len(m.items)-1]
}

// User adds a message of the user and opens its turn.
func (m *Meter) User(text string) {
	m.turn++
	label := firstLine(text)
	m.add(Item{Cat: CatMessages, Label: label, Text: "User: " + text})
	m.open = len(m.items) - 1
}

// Attach adds what the editor sends along with a message, such as the pinned open scene and selection.
func (m *Meter) Attach(label, text string) {
	if text != "" {
		m.add(Item{Cat: CatScene, Label: label, Text: text, Pinned: true})
	}
}

// Text adds the agent's answer to the messages of the turn.
func (m *Meter) Text(text string) {
	m.rev++
	if m.open < 0 || m.open >= len(m.items) {
		m.add(Item{Cat: CatMessages, Label: firstLine(text), Text: "Agent: " + text})
		m.open = len(m.items) - 1
		return
	}
	it := &m.items[m.open]
	it.Text += "\nAgent: " + text
	it.Est = Estimate(it.Text)
}

// Call adds a tool call the agent made: its arguments are the agent's output, its label waits for the result.
func (m *Meter) Call(tool, callID, args, label string) {
	m.rev++
	m.starts[callID] = call{tool: tool, label: label}
	if m.open >= 0 && m.open < len(m.items) {
		it := &m.items[m.open]
		it.Text += "\nTool " + tool + " " + args
		it.Est = Estimate(it.Text)
	}
}

// Result adds what a tool returned to the agent.
func (m *Meter) Result(tool, callID, result, detail string, failed bool) {
	c := m.starts[callID]
	delete(m.starts, callID)
	if tool == "" {
		tool = c.tool
	}
	label := c.label
	if label == "" {
		label = tool
	}
	m.add(Item{Cat: CategoryOf(tool), Label: label, Detail: detail, Tool: tool, CallID: callID, Text: result, Failed: failed})
}

// Usage is the context the model read for its last answer; the first one of a session measures the CLI's own prompt.
func (m *Meter) Usage(tokens int) {
	if tokens <= 0 {
		return
	}
	m.rev++
	if m.first {
		m.first = false
		ours := m.instr + m.tools
		for i := range m.items {
			ours += m.items[i].Est
		}
		m.base = max(0, tokens-ours)
	}
	m.Used = tokens
}

// Compacted is what the CLI did when it compacted the history: the items it summarized are gone, usage comes with the next answer.
func (m *Meter) Compacted() {
	m.rev++
	m.items = slices.DeleteFunc(m.items, func(it Item) bool { return !it.Pinned })
	m.open = -1
}

// Items are the contributions in the order they came.
func (m *Meter) Items() []Item {
	return m.items
}

// Item finds a contribution by id.
func (m *Meter) Item(id int) *Item {
	for i := range m.items {
		if m.items[i].ID == id {
			return &m.items[i]
		}
	}
	return nil
}

// Drop marks a contribution to leave out of the history the next turn rebuilds; Pin keeps it in full.
func (m *Meter) Drop(id int, on bool) {
	m.rev++
	if it := m.Item(id); it != nil {
		it.Dropped = on
		if on {
			it.Pinned = false
		}
	}
}

func (m *Meter) Pin(id int, on bool) {
	m.rev++
	if it := m.Item(id); it != nil {
		it.Pinned = on
		if on {
			it.Dropped = false
		}
	}
}

// Dropped is how many contributions wait to be left out, and their tokens.
func (m *Meter) Dropped() (int, int) {
	n, t := 0, 0
	for i := range m.items {
		if m.items[i].Dropped {
			n++
			t += m.items[i].Est
		}
	}
	return n, t
}

// Layout spreads the CLI's usage over the categories: fixed parts as measured, the editor's items scaled to fit, the rest to Messages.
func (m *Meter) Layout(dst *Layout) {
	*dst = Layout{Model: m.Model, Window: max(m.Window, 1), Buffer: CompactBuffer, Measured: !m.first}
	ours := 0
	for i := range m.items {
		ours += m.items[i].Est
	}
	fixed := m.base + m.tools + m.instr
	used := m.Used
	if m.first {
		used = fixed + ours
	}
	factor, rest := 1.0, 0
	if room := used - fixed; ours > 0 && room > 0 {
		factor = min(max(float64(room)/float64(ours), minScale), maxScale)
		rest = max(0, room-int(float64(ours)*factor))
	} else if room > 0 {
		rest = room
	}
	dst.Used = used
	dst.Rows[CatSystem].Tokens = m.base
	dst.Rows[CatTools].Tokens = m.tools
	dst.Rows[CatInstructions].Tokens = m.instr
	for i := range m.items {
		it := &m.items[i]
		it.Tokens = int(float64(it.Est) * factor)
		row := &dst.Rows[it.Cat]
		row.Tokens += it.Tokens
		row.Items = append(row.Items, i)
	}
	dst.Rows[CatMessages].Tokens += rest
	for c := range dst.Rows {
		dst.Rows[c].Cat = Category(c)
		items := dst.Rows[c].Items
		slices.SortFunc(items, func(a, b int) int { return cmp.Compare(m.items[b].Tokens, m.items[a].Tokens) })
	}
	dst.Free = max(0, dst.Window-used-dst.Buffer)
}

// Fraction is how full the context is, 0 to 1.
func (l *Layout) Fraction() float64 {
	return float64(l.Used) / float64(max(l.Window, 1))
}

// NearCompaction is true within a tenth of the window from the point where the CLI compacts.
func (l *Layout) NearCompaction() bool {
	return l.Used >= l.Window-l.Buffer-l.Window/10
}

// Projection is the history a rebuilt session starts from: messages in full, results as summaries, pinned ones in full, dropped ones left out.
func (m *Meter) Projection() string {
	var b strings.Builder
	b.WriteString("This continues an earlier session with you in the Aego editor. The history is shortened: tool results are summaries unless kept in full, and some were left out. Read again what you need.\n")
	turn := -1
	for i := range m.items {
		it := &m.items[i]
		if it.Dropped || it.Cat == CatScene && it.Tool == "" {
			continue
		}
		if it.Turn != turn {
			turn = it.Turn
			b.WriteString("\n")
		}
		switch {
		case it.Tool == "":
			b.WriteString(it.Text + "\n")
		case it.Pinned:
			b.WriteString("Result of " + it.Label + ":\n" + it.Text + "\n")
		case !it.Failed:
			line := "Result of " + it.Label
			if it.Detail != "" {
				line += ": " + it.Detail
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// Rebuilt keeps the pinned results and the projection the new session starts from, as one Messages item.
func (m *Meter) Rebuilt(projection string) {
	m.rev++
	kept := m.items[:0]
	for _, it := range m.items {
		if it.Pinned && it.Tool != "" {
			kept = append(kept, it)
		}
	}
	m.items = kept
	m.first, m.Used, m.open = true, 0, -1
	m.turn++
	it := m.add(Item{Cat: CatMessages, Label: i18n.Tr("editor.agent.ctx.earlier", "Earlier session, shortened"), Text: projection})
	for i := range m.items {
		if m.items[i].Pinned && m.items[i].Tool != "" {
			it.Est -= m.items[i].Est
		}
	}
	it.Est = max(it.Est, 1)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > 60 {
		r := []rune(s)
		s = string(r[:60]) + "…"
	}
	return s
}
