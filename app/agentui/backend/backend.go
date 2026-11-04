// Package backend runs Claude Code or Codex for the Agent panel behind one interface.
package backend

import (
	"encoding/json"
	"io"
)

type EventKind uint8

const (
	EvText EventKind = iota
	EvToolStart
	EvToolResult
	EvTurnEnd
	EvError
	// EvUsage reports Tokens, the context the model read for its last answer, and Model.
	EvUsage
	// EvCompact reports that the CLI compacted the history; Tokens is the context before it.
	EvCompact
)

// ToolPrefix is how Claude Code names the tools of the aego MCP server.
const ToolPrefix = "mcp__aego__"

type Event struct {
	Images [][]byte
	Kind   EventKind
	Text   string
	Tool   string
	CallID string
	Args   json.RawMessage
	Result string
	Failed bool
	Err    error
	// Delta appends Text verbatim to MessageID instead of starting a complete text block.
	Delta     bool
	MessageID string
	// Cost in US dollars and Millis of the answer, on EvTurnEnd.
	Cost   float64
	Millis int
	// Tokens on EvUsage and EvCompact; Window is the model's context size, on EvTurnEnd when the CLI reports it.
	Tokens int
	Window int
	Model  string
}

type Config struct {
	Binding string
	Dir     string
	Command string
	MCPURL  string
	Token   string
	System  string
	// Empty Model and Effort inherit the selected CLI's configuration.
	Model  string
	Effort string
	// Record gets every line the CLI writes, for recording sessions the tests replay.
	Record io.Writer
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
