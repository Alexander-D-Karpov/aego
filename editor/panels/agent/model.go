package agent

import (
	"time"

	"aego/editor/core/agentui"
	"aego/ui/core"
	"aego/ui/richtext"
	"aego/ui/widgets"
)

type blockKind uint8

const (
	blockUser blockKind = iota
	blockText
	// blockSteps folds read-only tool calls that follow each other into one row.
	blockSteps
	// blockFailed is one tool call that failed, with Retry and Ask Agent to Fix.
	blockFailed
	// blockCard is a write call and the card it proposed.
	blockCard
	blockQuestion
	blockError
	// blockTurn separates the answers to two messages.
	blockTurn
)

// step is one tool call; its look comes from the host, the raw result is not kept (11b5a).
type step struct {
	tool, call, args string
	look             agentui.Step
	done, failed     bool
	open, all        bool
	label            string
	fl               fit
}

type block struct {
	kind     blockKind
	text     string
	md       *richtext.Text
	parsed   string
	steps    []step
	card     int
	question int
	at       time.Time
	open     bool
	all      bool
	h, hw    float32
	// pick and other are a waiting question's chosen options and typed answer.
	pick  []bool
	other string
	lines []string
	fl    fit
}

// dirty makes the block measured again before the next draw.
func (b *block) dirty() {
	b.hw = -1
}

// fit is a string elided to a width, kept until the string or the width changes, so drawing a long transcript does not allocate.
type fit struct {
	src string
	w   float32
	out string
}

func (f *fit) get(ui *core.Context, s string, w float32) string {
	if s != f.src || w != f.w || f.out == "" && s != "" {
		f.src, f.w, f.out = s, w, widgets.Elide(ui, s, w)
	}
	return f.out
}

// stepLabel is "title · summary".
func stepLabel(s *step) string {
	if s.look.Summary == "" {
		return s.look.Title
	}
	return s.look.Title + " · " + s.look.Summary
}

func (p *Panel) last() *block {
	if n := len(p.blocks); n > 0 {
		return &p.blocks[n-1]
	}
	return nil
}

func (p *Panel) push(b block) *block {
	b.hw = -1
	p.blocks = append(p.blocks, b)
	return &p.blocks[len(p.blocks)-1]
}

// toolStart adds a call: a write call waits for its card, a read call joins the row of the calls before it.
func (p *Panel) toolStart(e agentui.Event) {
	s := step{tool: e.Tool, call: e.CallID, args: e.Args, look: p.d.Host.Describe(e.Tool, e.Args, "", false)}
	s.label = stepLabel(&s)
	if s.look.Write {
		p.push(block{kind: blockCard, steps: []step{s}, open: true})
		return
	}
	if b := p.last(); b != nil && b.kind == blockSteps {
		b.steps = append(b.steps, s)
		b.dirty()
		return
	}
	p.push(block{kind: blockSteps, steps: []step{s}})
}

// toolResult finishes the call: its look gets the summary and the laid-out result, and a failed read call moves into a row of its own.
func (p *Panel) toolResult(e agentui.Event) {
	for i := len(p.blocks) - 1; i >= 0; i-- {
		b := &p.blocks[i]
		for j := len(b.steps) - 1; j >= 0; j-- {
			s := &b.steps[j]
			if s.done || (e.CallID != "" && s.call != e.CallID) || (e.CallID == "" && s.tool != e.Tool) {
				continue
			}
			s.done, s.failed = true, e.Failed
			s.look = p.d.Host.Describe(s.tool, s.args, e.Result, e.Failed)
			s.label = stepLabel(s)
			b.dirty()
			if e.Failed && b.kind == blockSteps {
				failed := *s
				b.steps = append(b.steps[:j], b.steps[j+1:]...)
				if len(b.steps) == 0 {
					*b = block{kind: blockFailed, steps: []step{failed}, hw: -1}
				} else {
					p.insert(i+1, block{kind: blockFailed, steps: []step{failed}})
				}
			}
			return
		}
	}
}

func (p *Panel) insert(at int, b block) {
	b.hw = -1
	p.blocks = append(p.blocks, block{})
	copy(p.blocks[at+1:], p.blocks[at:])
	p.blocks[at] = b
}

// attach gives a new card to the write call that proposed it, or a block of its own.
func (p *Panel) attach(c *agentui.Card) {
	for i := len(p.blocks) - 1; i >= 0; i-- {
		b := &p.blocks[i]
		if b.kind == blockCard && b.card == 0 && len(b.steps) == 1 && b.steps[0].tool == toolName(c.Command) {
			b.card = c.ID
			b.dirty()
			return
		}
	}
	look := p.d.Host.Describe(toolName(c.Command), c.Args, "", false)
	s := step{tool: toolName(c.Command), args: c.Args, look: look, done: true}
	s.label = stepLabel(&s)
	p.push(block{kind: blockCard, card: c.ID, steps: []step{s}, open: c.State == agentui.Pending})
}

// toolName is the tool form of a command name: scene.add-node is scene_add_node.
func toolName(cmd string) string {
	b := []byte(cmd)
	for i, c := range b {
		if c == '.' || c == '-' {
			b[i] = '_'
		}
	}
	return string(b)
}

// mergeLimit is how long a message grows before the next text starts a block of its own, so new text lays out only the last block.
const mergeLimit = 2048

// text joins answer text into one message until the next tool call.
func (p *Panel) text(s string) {
	if b := p.last(); b != nil && b.kind == blockText && len(b.text) < mergeLimit {
		b.text += "\n\n" + s
		b.dirty()
		return
	}
	p.push(block{kind: blockText, text: s})
}
