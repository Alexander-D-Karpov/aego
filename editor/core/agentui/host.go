// Package agentui is the contract between the agent panel and the app that runs the model and its tools (D343).
package agentui

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
	ID          int
	Command     string
	Args        string
	Changes     []Change
	Notes       []string
	Problems    []string
	Destructive bool
	State       State
	Err         string
	CanUndo     bool
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
}
