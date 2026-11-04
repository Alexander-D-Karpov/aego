// Package agentui is the contract between the agent panel and the app that runs the model and its tools (D343).
package agentui

import (
	"context"

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
	// EvUsage is the context the model read for its last answer, in Tokens, and the Model.
	EvUsage
	// EvCompact is the CLI compacting the history; Tokens is the context before it.
	EvCompact
)

type Event struct {
	Images    [][]byte
	ImageKeys []string
	Kind      EventKind
	Text      string
	Tool      string
	CallID    string
	Args      string
	Result    string
	Failed    bool
	// Delta appends Text verbatim to MessageID; ordinary EvText contains a complete block.
	Delta     bool
	MessageID string
	// Cost in US dollars and Millis of the answer, on EvTurnEnd when the backend reports them.
	Cost   float64
	Millis int
	// Tokens on EvUsage and EvCompact; Window is the model's context size on EvTurnEnd when known.
	Tokens int
	Window int
	Model  string
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
	PreviewImages [][]byte
	ID            int
	Command       string
	Args          string
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

// Host runs the model and its tools for the panel. Available may run off the main thread;
// all other methods are called on the main thread.
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
	// ContextBase is what the model reads before the first message: the editor's guide and the tool definitions; empty before the first Send.
	ContextBase() (instructions, tools string)
	// Budgets are the limits the read tools trim their results to; SetBudgets changes them for the next calls.
	Budgets() Budgets
	SetBudgets(b Budgets)
}

// ProviderSettings are local editor preferences, never project assets or credentials.
// Empty command, model and effort inherit the selected CLI's defaults.
type ProviderSettings struct {
	Provider string
	Command  string
	Model    string
	Effort   string
}

// ProviderHost is optional so injected test and replay hosts keep the same contract.
type ProviderHost interface {
	Provider() string
	SetProvider(string) error
}

// ModelHost controls overrides for the selected provider. Changes start a new conversation.
type ModelHost interface {
	Model() string
	SetModel(string) error
	Effort() string
	SetEffort(string) error
}

// ModelOption is an advertised model and its supported reasoning levels.
type ModelOption struct {
	ID, Label, Description string
	Efforts                []string
}

// ModelCatalogHost is optional. ListModels may run off the main thread; its
// arguments are a snapshot, so changing providers cannot affect an old probe.
type ModelCatalogHost interface {
	ListModels(ctx context.Context, provider, command string) ([]ModelOption, error)
}

// Budgets cap what read tools return, so a large scene or a broad search does not fill the context.
type Budgets struct {
	// TreeNodes and TreeDepth cap scene_tree; DocResults caps docs_search; ScriptLines is the longest script read whole.
	TreeNodes   int
	TreeDepth   int
	DocResults  int
	ScriptLines int
}

// DefaultBudgets are the limits a new editor starts with.
var DefaultBudgets = Budgets{TreeNodes: 200, TreeDepth: 8, DocResults: 10, ScriptLines: 400}

type Connection struct {
	Provider, Model, Project, Document, Thread, State, Last, Vision, Probe string
	Tools                                                                  int
}

type SceneHost interface {
	ExplainScene(text string) error
	CheckConnection() error
	Diagnostics() Connection
}
