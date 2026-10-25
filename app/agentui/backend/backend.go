// Package backend runs a model for the Agent panel behind one interface: the Claude Code CLI, or a scripted fake for tests (D341).
package backend

import "encoding/json"

type EventKind uint8

const (
	EvText EventKind = iota
	EvToolStart
	EvToolResult
	EvTurnEnd
	EvError
)

// ToolPrefix is how Claude Code names the tools of the aego MCP server.
const ToolPrefix = "mcp__aego__"

type Event struct {
	Kind   EventKind
	Text   string
	Tool   string
	CallID string
	Args   json.RawMessage
	Result string
	Failed bool
	Err    error
	// Cost in US dollars and Millis of the answer, on EvTurnEnd.
	Cost   float64
	Millis int
}

type Config struct {
	Dir     string
	Command string
	MCPURL  string
	Token   string
	System  string
}

type Conversation interface {
	Send(text string) error
	Events() <-chan Event
	Cancel()
	Close() error
}

type Backend interface {
	Name() string
	Available(cfg Config) (string, error)
	Start(cfg Config) (Conversation, error)
}
