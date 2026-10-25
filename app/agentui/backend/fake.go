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

// Fake plays scripted turns, one per Send, for tests.
type Fake struct {
	Turns [][]Step
}

func (f *Fake) Name() string {
	return "fake"
}

func (f *Fake) Available(Config) (string, error) {
	return "fake", nil
}

func (f *Fake) Start(cfg Config) (Conversation, error) {
	return &fakeConv{turns: f.Turns, client: NewClient(cfg.MCPURL, cfg.Token), events: make(chan Event, 64), stop: make(chan struct{})}, nil
}

type fakeConv struct {
	mu     sync.Mutex
	once   sync.Once
	turns  [][]Step
	n      int
	client *Client
	events chan Event
	stop   chan struct{}
}

func (c *fakeConv) Send(string) error {
	c.mu.Lock()
	if c.n >= len(c.turns) {
		c.mu.Unlock()
		return errors.New("the fake backend has no more turns")
	}
	turn := c.turns[c.n]
	c.n++
	c.mu.Unlock()
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
