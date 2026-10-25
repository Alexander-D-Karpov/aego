// Package presence is where others work in the project: the agent now, collaborators later (11b5b, D343).
package presence

import (
	"aego/core/ids"
	"aego/core/mathx"
)

const (
	// Linger is how long a marker stays after the last call that moved it.
	Linger = 5.0
	// FlashTime is how long a changed field, node or line is lit.
	FlashTime = 2.0
	// LineTime is how long changed lines stay marked in the gutter.
	LineTime = 10.0
)

// Spot is one place: a document, and in it a node, a component and a field, or a line of a script.
type Spot struct {
	Doc   string
	Node  ids.NodeID
	Type  string
	Field string
	Line  int
	Panel string
}

// Who is one presence: its name and where it was last, at what time.
type Who struct {
	Name string
	Spot Spot
	At   float64
}

type FlashKind uint8

const (
	Changed FlashKind = iota
	Added
	Removed
)

// Flash lights a changed field, an added node or component, or a removed node's ghost.
type Flash struct {
	Spot Spot
	Kind FlashKind
	At   float64
	// Ghost is the world box of a removed node, drawn fading where it was.
	Ghost mathx.Rect
	Name  string
}

// Lines marks lines of a script the agent changed, or will change when a card is applied.
type Lines struct {
	Doc      string
	From, To int
	At       float64
	// Pending holds the text a waiting card would put there, drawn as ghost lines.
	Pending []string
}

// Edit is a field a waiting card will change, so the Inspector warns before the user edits it too.
type Edit struct {
	Spot Spot
	Card int
}

type Model struct {
	// Following is true while the editor follows the agent: the Inspector shows the agent's node.
	Following bool
	who       []Who
	flashes   []Flash
	lines     []Lines
	edits     []Edit
	rev       uint64
}

func New() *Model {
	return &Model{}
}

// Rev changes whenever something is added, so panels know when to look again.
func (m *Model) Rev() uint64 {
	return m.rev
}

// Move puts name at s.
func (m *Model) Move(name string, s Spot, now float64) {
	m.rev++
	for i := range m.who {
		if m.who[i].Name == name {
			m.who[i].Spot, m.who[i].At = s, now
			return
		}
	}
	m.who = append(m.who, Who{Name: name, Spot: s, At: now})
}

// Where is where name is, while its marker lingers.
func (m *Model) Where(name string, now float64) (Spot, float64, bool) {
	for i := range m.who {
		w := &m.who[i]
		if w.Name == name && now-w.At < Linger {
			return w.Spot, w.At, true
		}
	}
	return Spot{}, 0, false
}

// Present lists everyone whose marker still shows.
func (m *Model) Present(now float64, dst []Who) []Who {
	for _, w := range m.who {
		if now-w.At < Linger {
			dst = append(dst, w)
		}
	}
	return dst
}

// At reports whether someone is on the node of doc, and how long ago they got there.
func (m *Model) At(doc string, node ids.NodeID, now float64) (float64, bool) {
	for i := range m.who {
		w := &m.who[i]
		if w.Spot.Doc == doc && w.Spot.Node == node && !node.IsZero() && now-w.At < Linger {
			return now - w.At, true
		}
	}
	return 0, false
}

func (m *Model) Light(f Flash) {
	m.rev++
	m.flashes = append(m.flashes, f)
}

// FieldLight is how lit a field is, 1 when it just changed and 0 after FlashTime.
func (m *Model) FieldLight(doc string, node ids.NodeID, typ, field string, now float64) float32 {
	var best float32
	for i := range m.flashes {
		f := &m.flashes[i]
		s := &f.Spot
		if s.Doc != doc || s.Node != node || s.Type != typ || s.Field != field {
			continue
		}
		best = max(best, fade(now-f.At, FlashTime))
	}
	return best
}

// NodeLight is how lit a node or one of its components is: added ones light whole.
func (m *Model) NodeLight(doc string, node ids.NodeID, typ string, now float64) float32 {
	var best float32
	for i := range m.flashes {
		f := &m.flashes[i]
		s := &f.Spot
		if f.Kind != Added || s.Doc != doc || s.Node != node || s.Type != typ || s.Field != "" {
			continue
		}
		best = max(best, fade(now-f.At, FlashTime))
	}
	return best
}

// Ghosts calls fn for the removed nodes of doc that still fade.
func (m *Model) Ghosts(doc string, now float64, fn func(f *Flash, a float32)) {
	for i := range m.flashes {
		f := &m.flashes[i]
		if f.Kind == Removed && f.Spot.Doc == doc {
			if a := fade(now-f.At, FlashTime); a > 0 {
				fn(f, a)
			}
		}
	}
}

func fade(age, span float64) float32 {
	if age < 0 || age >= span {
		return 0
	}
	return float32(1 - age/span)
}

func (m *Model) Mark(l Lines) {
	m.rev++
	for i := range m.lines {
		if m.lines[i].Doc == l.Doc && m.lines[i].From == l.From {
			m.lines[i] = l
			return
		}
	}
	m.lines = append(m.lines, l)
}

// Unmark drops the ghost lines of doc that start at from, when their card is decided.
func (m *Model) Unmark(doc string, from int) {
	for i := range m.lines {
		if m.lines[i].Doc == doc && m.lines[i].From == from && m.lines[i].Pending != nil {
			m.rev++
			m.lines = append(m.lines[:i], m.lines[i+1:]...)
			return
		}
	}
}

// Lines calls fn for the marked lines of doc.
func (m *Model) LinesOf(doc string, now float64, fn func(l *Lines, a float32)) {
	for i := range m.lines {
		l := &m.lines[i]
		if l.Doc != doc {
			continue
		}
		if l.Pending != nil {
			fn(l, 1)
		} else if a := fade(now-l.At, LineTime); a > 0 {
			fn(l, a)
		}
	}
}

// SetEdits are the fields the waiting cards will change.
func (m *Model) SetEdits(list []Edit) {
	m.edits = append(m.edits[:0], list...)
}

// Editing reports whether a waiting card will change this field.
func (m *Model) Editing(doc string, node ids.NodeID, typ, field string) bool {
	for i := range m.edits {
		s := &m.edits[i].Spot
		if s.Doc == doc && s.Node == node && s.Type == typ && (s.Field == field || s.Field == "") {
			return true
		}
	}
	return false
}

// Clear forgets every marker and light.
func (m *Model) Clear() {
	m.rev++
	m.who, m.flashes, m.lines, m.edits = m.who[:0], m.flashes[:0], m.lines[:0], m.edits[:0]
}

// Prune drops lights that have faded, so the lists stay short.
func (m *Model) Prune(now float64) {
	n := 0
	for _, f := range m.flashes {
		if now-f.At < FlashTime {
			m.flashes[n] = f
			n++
		}
	}
	m.flashes = m.flashes[:n]
	n = 0
	for _, l := range m.lines {
		if l.Pending != nil || now-l.At < LineTime {
			m.lines[n] = l
			n++
		}
	}
	m.lines = m.lines[:n]
}
