package backend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Claude runs the Claude Code CLI with stream-json in and out; only the aego MCP tools are available to it.
type Claude struct{}

func (Claude) Name() string {
	return "claude"
}

func binary(cfg Config) string {
	if cfg.Command != "" {
		return cfg.Command
	}
	return "claude"
}

func (Claude) Available(cfg Config) (string, error) {
	out, err := exec.Command(binary(cfg), "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func mcpConfig(cfg Config) (string, error) {
	f, err := os.CreateTemp("", "aego-mcp-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	conf := map[string]any{"mcpServers": map[string]any{"aego": map[string]any{
		"type": "http", "url": cfg.MCPURL,
		"headers": map[string]string{"Authorization": "Bearer " + cfg.Token},
	}}}
	if err := json.NewEncoder(f).Encode(conf); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// args turns off the built-in tools and every other MCP server, so the model reaches the project only through aego.
func args(cfg Config, conf string) []string {
	out := []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose",
		"--strict-mcp-config", "--mcp-config", conf, "--tools", "", "--allowedTools", "mcp__aego"}
	if cfg.System != "" {
		out = append(out, "--append-system-prompt", cfg.System)
	}
	return out
}

func (Claude) Start(cfg Config) (Conversation, error) {
	conf, err := mcpConfig(cfg)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary(cfg), args(cfg, conf)...)
	cmd.Dir = cfg.Dir
	cmd.Env = append(os.Environ(), "MCP_TOOL_TIMEOUT=660000")
	c := &claudeConv{cmd: cmd, conf: conf, events: make(chan Event, 64)}
	cmd.Stderr = &c.stderr
	if c.in, err = cmd.StdinPipe(); err != nil {
		os.Remove(conf)
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		os.Remove(conf)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		os.Remove(conf)
		return nil, err
	}
	go c.read(out)
	return c, nil
}

type claudeConv struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	conf   string
	stderr bytes.Buffer
	events chan Event
	mu     sync.Mutex
	closed bool
}

func (c *claudeConv) Send(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("the conversation is closed")
	}
	msg := map[string]any{"type": "user", "message": map[string]any{"role": "user",
		"content": []any{map[string]any{"type": "text", "text": text}}}}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(b, '\n'))
	return err
}

func (c *claudeConv) read(r io.Reader) {
	defer close(c.events)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for sc.Scan() {
		for _, e := range Decode(sc.Bytes()) {
			c.events <- e
		}
	}
	if err := c.cmd.Wait(); err != nil {
		msg := strings.TrimSpace(c.stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		c.events <- Event{Kind: EvError, Err: errors.New(msg)}
	}
}

func (c *claudeConv) Events() <-chan Event {
	return c.events
}

func (c *claudeConv) Cancel() {
	if c.cmd.Process != nil {
		c.cmd.Process.Signal(os.Interrupt)
	}
}

func (c *claudeConv) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()
	c.in.Close()
	os.Remove(c.conf)
	return nil
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

type streamLine struct {
	Type    string  `json:"type"`
	IsError bool    `json:"is_error"`
	Result  string  `json:"result"`
	Cost    float64 `json:"total_cost_usd"`
	Millis  int     `json:"duration_ms"`
	Message struct {
		Content []block `json:"content"`
	} `json:"message"`
}

func textOf(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	json.Unmarshal(raw, &parts)
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// Decode turns one stream-json line of Claude Code into events; lines it does not know give none.
func Decode(line []byte) []Event {
	var l streamLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}
	var out []Event
	switch l.Type {
	case "assistant":
		for _, b := range l.Message.Content {
			switch b.Type {
			case "text":
				out = append(out, Event{Kind: EvText, Text: b.Text})
			case "tool_use":
				out = append(out, Event{Kind: EvToolStart, Tool: strings.TrimPrefix(b.Name, ToolPrefix), CallID: b.ID, Args: b.Input})
			}
		}
	case "user":
		for _, b := range l.Message.Content {
			if b.Type == "tool_result" {
				out = append(out, Event{Kind: EvToolResult, CallID: b.ToolUseID, Result: textOf(b.Content), Failed: b.IsError})
			}
		}
	case "result":
		if l.IsError {
			out = append(out, Event{Kind: EvError, Err: errors.New(l.Result)})
		}
		out = append(out, Event{Kind: EvTurnEnd, Cost: l.Cost, Millis: l.Millis})
	}
	return out
}
