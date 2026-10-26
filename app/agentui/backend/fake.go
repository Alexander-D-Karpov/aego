package backend

import (
	"encoding/json"
	"errors"
	"strconv"
	"sync"
)

// Step is one move of a scripted turn: text, or a tool called through the MCP server.
type Step struct {
	Text string
	Tool string
	Args map[string]any
}

// Fake plays scripted turns, one per Send, for tests; a new conversation goes on with the next turn, and Sent keeps every message.
type Fake struct {
	Turns [][]Step
	mu    sync.Mutex
	n     int
	sent  []string
}

// Sent is the messages the conversations got, in order.
func (f *Fake) Sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func (f *Fake) Name() string {
	return "fake"
}

func (f *Fake) Available(Config) (string, error) {
	return "fake", nil
}

func (f *Fake) Start(cfg Config) (Conversation, error) {
	return &fakeConv{f: f, client: NewClient(cfg.MCPURL, cfg.Token), events: make(chan Event, 64), stop: make(chan struct{})}, nil
}

type fakeConv struct {
	f      *Fake
	once   sync.Once
	client *Client
	events chan Event
	stop   chan struct{}
}

func (c *fakeConv) Send(text string) error {
	f := c.f
	f.mu.Lock()
	f.sent = append(f.sent, text)
	if f.n >= len(f.Turns) {
		f.mu.Unlock()
		return errors.New("the fake backend has no more turns")
	}
	turn := f.Turns[f.n]
	f.n++
	f.mu.Unlock()
	go c.play(turn)
	return nil
}

func (c *fakeConv) play(turn []Step) {
	for i, st := range turn {
		select {
		case <-c.stop:
			c.events <- Event{Kind: EvTurnEnd}
			return
		default:
		}
		if st.Tool == "" {
			c.events <- Event{Kind: EvText, Text: st.Text}
			continue
		}
		id := strconv.Itoa(i + 1)
		args, _ := json.Marshal(st.Args)
		c.events <- Event{Kind: EvToolStart, Tool: st.Tool, CallID: id, Args: args}
		text, failed, err := c.client.Call(st.Tool, st.Args)
		if err != nil {
			c.events <- Event{Kind: EvError, Err: err}
			return
		}
		c.events <- Event{Kind: EvToolResult, Tool: st.Tool, CallID: id, Result: text, Failed: failed}
	}
	c.events <- Event{Kind: EvTurnEnd}
}

func (c *fakeConv) Events() <-chan Event {
	return c.events
}

func (c *fakeConv) Cancel() {
	c.once.Do(func() { close(c.stop) })
}

func (c *fakeConv) Close() error {
	c.Cancel()
	return nil
}
