package agentui

// Step is a tool call as the panel shows it: an icon, a title from its arguments and a summary of its result (11b5a).
type Step struct {
	Icon    string
	Title   string
	Summary string
	// Write steps change the project and come with a card; the others are folded into one row.
	Write  bool
	Detail Detail
}

type DetailKind uint8

const (
	DetailNone DetailKind = iota
	// DetailList is search results or assets: an icon, a title and a summary per row.
	DetailList
	// DetailTree is a node tree; Depth indents a row.
	DetailTree
	// DetailChips are names grouped by Group, such as components by category.
	DetailChips
	// DetailGrid is assets as tiles.
	DetailGrid
	// DetailTable is name and value rows.
	DetailTable
	// DetailDiff is changes: node, field, before and after.
	DetailDiff
	// DetailLog is log lines and problems; Level 1 is a warning, 2 an error.
	DetailLog
	// DetailText is plain or code text.
	DetailText
)

type Row struct {
	Icon   string
	Title  string
	Text   string
	Group  string
	Depth  int
	Before string
	After  string
	Level  int
	// Link opens what the row names: doc:<id>, node:<file>#<id>, asset:<path> or file:<path>:<line>.
	Link string
}

type Detail struct {
	Kind DetailKind
	Rows []Row
	Text string
}
