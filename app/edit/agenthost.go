package edit

import (
	"encoding/json"

	"aego/app/agent"
	"aego/app/agentui/backend"
	"aego/app/edit/agentbridge"
	"aego/editor/core/agentui"
)

// agentHost runs the model and the bridge for the Agent panel; the bridge and the process start on the first message (D343).
type agentHost struct {
	backend backend.Backend
	config  func() agentbridge.Config
	command func() string
	setCmd  func(string)
	bridge  *agentbridge.Bridge
	conv    backend.Conversation
	busy    bool
	auto    bool
	static  map[int]agentui.Card
}

func newAgentHost(b backend.Backend, config func() agentbridge.Config, command func() string, set func(string)) *agentHost {
	if b == nil {
		b = backend.Claude{}
	}
	return &agentHost{backend: b, config: config, command: command, setCmd: set, static: map[int]agentui.Card{}}
}

func (h *agentHost) Available(cmd string) (string, error) {
	return h.backend.Available(backend.Config{Command: cmd})
}

func (h *agentHost) Command() string {
	if c := h.command(); c != "" {
		return c
	}
	return "claude"
}

func (h *agentHost) SetCommand(path string) {
	if path == "claude" {
		path = ""
	}
	h.setCmd(path)
	if h.conv != nil {
		h.conv.Close()
		h.conv, h.busy = nil, false
	}
}

func (h *agentHost) Running() bool {
	return h.conv != nil
}

func (h *agentHost) Busy() bool {
	return h.busy
}

func (h *agentHost) start() error {
	if h.bridge == nil {
		b, err := agentbridge.Start(h.config())
		if err != nil {
			return err
		}
		b.SetAutoApply(h.auto)
		h.bridge = b
	}
	if h.conv == nil {
		c, err := h.backend.Start(h.bridge.Backend())
		if err != nil {
			return err
		}
		h.conv = c
	}
	return nil
}

func (h *agentHost) Send(text string) error {
	if err := h.start(); err != nil {
		return err
	}
	if err := h.conv.Send(text); err != nil {
		return err
	}
	h.busy = true
	return nil
}

func (h *agentHost) Cancel() {
	if h.conv != nil {
		h.conv.Cancel()
	}
	if h.bridge != nil {
		h.bridge.RejectAll()
	}
}

func (h *agentHost) Close() {
	if h.conv != nil {
		h.conv.Close()
		h.conv = nil
	}
	if h.bridge != nil {
		h.bridge.Close()
		h.bridge = nil
	}
	h.busy = false
}

func eventOf(e backend.Event) agentui.Event {
	out := agentui.Event{Text: e.Text, Tool: e.Tool, CallID: e.CallID, Args: string(e.Args), Result: e.Result, Failed: e.Failed}
	switch e.Kind {
	case backend.EvText:
		out.Kind = agentui.EvText
	case backend.EvToolStart:
		out.Kind = agentui.EvToolStart
	case backend.EvToolResult:
		out.Kind = agentui.EvToolResult
	case backend.EvTurnEnd:
		out.Kind = agentui.EvTurnEnd
	default:
		out.Kind = agentui.EvError
		if e.Err != nil {
			out.Text = e.Err.Error()
		}
	}
	return out
}

// Poll takes the events waiting in the conversation without blocking.
func (h *agentHost) Poll(dst []agentui.Event) []agentui.Event {
	for h.conv != nil {
		select {
		case e, ok := <-h.conv.Events():
			if !ok {
				h.conv.Close()
				h.conv, h.busy = nil, false
				return append(dst, agentui.Event{Kind: agentui.EvError, Text: "the agent process ended; the next message starts a new one"})
			}
			if e.Kind == backend.EvTurnEnd || e.Kind == backend.EvError {
				h.busy = false
			}
			dst = append(dst, eventOf(e))
		default:
			return dst
		}
	}
	return dst
}

func stateOf(s agentbridge.State) agentui.State {
	switch s {
	case agentbridge.Approved:
		return agentui.Approved
	case agentbridge.Applied:
		return agentui.Applied
	case agentbridge.Rejected:
		return agentui.Rejected
	case agentbridge.Expired:
		return agentui.Expired
	case agentbridge.Failed:
		return agentui.Failed
	case agentbridge.Undone:
		return agentui.Undone
	}
	return agentui.Pending
}

func cardOf(p *agentbridge.Proposal) agentui.Card {
	c := agentui.Card{ID: p.ID, Command: p.Command, Destructive: p.Destructive}
	if b, err := json.Marshal(p.Args); err == nil {
		c.Args = string(b)
	}
	if out := p.Preview; out != nil {
		for _, ch := range out.Changes {
			c.Changes = append(c.Changes, agentui.Change{Op: ch.Op, Node: ch.Node, Path: ch.Path, Before: ch.Before, After: ch.After})
		}
		c.Notes = append(c.Notes, out.Notes...)
		c.Problems = problemTexts(out)
	}
	return c
}

func problemTexts(out *agent.Output) []string {
	var list []string
	for _, p := range out.Problems {
		level := "warning: "
		if p.Error {
			level = "error: "
		}
		list = append(list, level+p.Message)
	}
	return list
}

// Cards are the bridge's proposals; their fixed parts are built once.
func (h *agentHost) Cards(dst []agentui.Card) []agentui.Card {
	if h.bridge == nil {
		return dst
	}
	for _, p := range h.bridge.Proposals() {
		c, ok := h.static[p.ID]
		if !ok {
			c = cardOf(p)
			h.static[p.ID] = c
		}
		c.State, c.Err, c.CanUndo = stateOf(p.State), p.Err, h.bridge.CanUndo(p.ID)
		dst = append(dst, c)
	}
	return dst
}

func (h *agentHost) Decide(id int, apply bool) {
	if h.bridge != nil {
		h.bridge.Decide(id, apply)
	}
}

func (h *agentHost) Undo(id int) error {
	if h.bridge == nil {
		return nil
	}
	return h.bridge.Undo(id)
}

func (h *agentHost) AutoApply() bool {
	return h.auto
}

func (h *agentHost) SetAutoApply(on bool) {
	h.auto = on
	if h.bridge != nil {
		h.bridge.SetAutoApply(on)
	}
}

var _ agentui.Host = (*agentHost)(nil)
