// Package agentui is the contract between the agent panel and the app that runs the model and its tools (D343).
package agentui

import (
	"aego/core/ids"
	"aego/schema"
)

type EventKind uint8

const (
	EvText EventKind = iota
	EvToolStart
	EvToolResult
	EvTurnEnd
	EvError
)

type Event struct {
	Kind   EventKind
	Text   string
	Tool   string
	CallID string
	Args   string
	Result string
	Failed bool
	// Cost in US dollars and Millis of the answer, on EvTurnEnd when the backend reports them.
	Cost   float64
	Millis int
}

type State uint8

const (
	Pending State = iota
	Approved
	Applied
	Rejected
	Expired
	Failed
	Undone
)

var stateNames = [...]string{"pending", "approved", "applied", "rejected", "expired", "failed", "undone"}

func (s State) String() string {
	if int(s) < len(stateNames) {
		return stateNames[s]
	}
	return "unknown"
}

func ParseState(s string) State {
	for i, n := range stateNames {
		if n == s {
			return State(i)
		}
	}
	return Pending
}

type Change struct {
	Op     string
	Node   string
	Path   string
	Before string
	After  string
	// NodeID, Type and Field locate the change for follow mode; zero when the change is not a field of a node.
	NodeID ids.NodeID
	Type   schema.TypeID
	Field  schema.FieldID
	// Line is the first changed line of a script edit, 1-based.
	Line int
}

func (c Change) String() string {
	line := c.Op + " " + c.Node
	if c.Path != "" {
		line += " " + c.Path
	}
	if c.Before != "" || c.After != "" {
		line += ": " + c.Before + " → " + c.After
	}
	return line
}

// Card is a write proposal as the panel shows it.
type Card struct {
	ID      int
	Command string
	Args    string
	// File is the document the card changes, relative to the project.
	File    string
	Changes []Change
	// Applied are the changes as applied, with the ids of created nodes; empty until the card is applied.
	Applied []Change
	// Rows are the changes as the panel shows them: names instead of ids, Inspector labels instead of type paths.
	Rows        []Row
	Notes       []string
	Problems    []string
	Destructive bool
	State       State
	Err         string
	CanUndo     bool
}

// Option is one answer the agent offers to its question.
type Option struct {
	Label  string
	Detail string
}

// Question is the agent asking the user to choose; the call waits for the answer (ask_user).
type Question struct {
	ID       int
	Text     string
	Options  []Option
	Multiple bool
	// Other lets the user type an answer of their own.
	Other  bool
	State  State
	Answer string
}

// Host runs the model and its tools for the panel; every method is called on the main thread.
type Host interface {
	Available(command string) (string, error)
	Command() string
	SetCommand(path string)
	Running() bool
	Busy() bool
	Send(text string) error
	Cancel()
	Close()
	Poll(dst []Event) []Event
	Cards(dst []Card) []Card
	Decide(id int, apply bool)
	Undo(id int) error
	AutoApply() bool
	SetAutoApply(on bool)
	// PlanOnly refuses every write tool, so the agent describes changes instead of making them.
	PlanOnly() bool
	SetPlanOnly(on bool)
	// Reset ends the conversation; the next message starts a new one.
	Reset()
	Questions(dst []Question) []Question
	// Answer picks options of a waiting question by index, or answers it with text.
	Answer(id int, picks []int, text string)
	// Describe is how a tool call looks in the panel; result is empty while the call runs.
	Describe(tool, args, result string, failed bool) Step
}
