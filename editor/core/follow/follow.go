// Package follow leads the editor after the agent: each tool call opens the document, selects and frames the node and lights what changed (11b5b, D343).
package follow

import (
	"encoding/json"
	"path"
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/presence"
	"aego/editor/core/selection"
	"aego/schema"
)

// Agent is the name of the agent in the presence model.
const Agent = "agent"

// DefaultResume is how long the user may be idle before following goes on by itself.
const DefaultResume = 30.0

// Nav is what the editor does when following: the shell implements it over its panels.
type Nav interface {
	// OpenScene activates the scene or prefab, opening it when it is closed; false when it cannot.
	OpenScene(rel string) (selection.DocID, bool)
	// Node finds a node of an open document by the agent's address: a path from the root or #id.
	Node(rel, addr string) (ids.NodeID, bool)
	// Reveal opens the node's parents in the Hierarchy and scrolls to it.
	Reveal(rel string, node ids.NodeID)
	// Frame moves the Scene View camera to the node smoothly.
	Frame(rel string, node ids.NodeID)
	// Inspect shows the node in the Inspector, scrolled to the component when one is named.
	Inspect(rel string, node ids.NodeID, comp string)
	// OpenScript shows the script at a line without taking the keyboard.
	OpenScript(rel string, line int)
	// ScriptLine is the line of the first place of text in an open script, 0 when it is not there.
	ScriptLine(rel, text string) int
	// Bounds is a node's world box, to draw where a removed node was.
	Bounds(rel string, node ids.NodeID) (mathx.Rect, bool)
	ShowProblems()
	ShowConsole()
	// OpenData opens a data asset or table at a row.
	OpenData(rel, row string)
	// TypeName is the full name of a component type by its id.
	TypeName(id schema.TypeID) string
	FieldName(typ schema.TypeID, field schema.FieldID) string
}

type Service struct {
	Model *presence.Model
	// Sel is the agent's selection, a second set that Delete, Copy and the Inspector's own selection never read.
	Sel *selection.Service
	Nav Nav
	// Resume is how long after the user's last action following goes on; 0 never.
	Resume float64

	on, paused bool
	lastUser   float64
	seen       map[int]agentui.State
	deferred   []deferred
	edits      []presence.Edit
	ghosts     []ghost
}

// ghost is the box of a node a waiting card removes, kept to fade where it was once the card is applied.
type ghost struct {
	card  int
	flash presence.Flash
}

// deferred is the user's edit of a field a waiting card also changes; it runs again after the card, so it is not lost.
type deferred struct {
	spot presence.Spot
	run  func()
}

func New(m *presence.Model, sel *selection.Service, nav Nav) *Service {
	return &Service{Model: m, Sel: sel, Nav: nav, Resume: DefaultResume, seen: map[int]agentui.State{}}
}

func (s *Service) On() bool {
	return s.on
}

func (s *Service) SetOn(on bool) {
	s.on, s.paused = on, false
}

// Paused reports whether the user's own work holds following until they resume it or stay idle.
func (s *Service) Paused() bool {
	return s.on && s.paused
}

// Active reports whether the editor follows the agent now.
func (s *Service) Active() bool {
	return s.on && !s.paused
}

func (s *Service) ResumeNow() {
	s.paused = false
}

// UserActive pauses following: the user clicked, typed, scrolled or selected.
func (s *Service) UserActive(now float64) {
	s.lastUser = now
	if s.on {
		s.paused = true
	}
}

// Tick resumes following after the user has been idle for Resume seconds and lets faded lights go.
func (s *Service) Tick(now float64) {
	if s.paused && s.Resume > 0 && now-s.lastUser >= s.Resume {
		s.paused = false
	}
	s.Model.Following = s.Active()
	s.Model.Prune(now)
}

type args struct {
	File   string `json:"file"`
	Node   string `json:"node"`
	Comp   string `json:"comp"`
	Path   string `json:"path"`
	Parent string `json:"parent"`
	Old    string `json:"old"`
	Row    string `json:"row"`
	ID     string `json:"id"`
}

func scene(rel string) bool {
	switch path.Ext(rel) {
	case ".aescene", ".aeprefab":
		return true
	}
	return false
}

// Step moves the agent's marker to where a tool call works and, while following, takes the editor there.
func (s *Service) Step(tool, raw string, now float64) {
	sp, ok := s.resolve(tool, raw, s.Active())
	if !ok {
		return
	}
	s.Model.Move(Agent, sp, now)
	if s.Active() {
		s.GoTo(sp)
	}
}

// Go takes the editor to where a tool call worked, once, whether or not it follows: a click on the timeline.
func (s *Service) Go(tool, raw string) {
	if sp, ok := s.resolve(tool, raw, true); ok {
		s.GoTo(sp)
	}
}

// resolve is the place a call works on; open opens its scene first, so its node can be found.
func (s *Service) resolve(tool, raw string, open bool) (presence.Spot, bool) {
	var a args
	_ = json.Unmarshal([]byte(raw), &a)
	group, _, _ := strings.Cut(tool, "_")
	sp := presence.Spot{Doc: a.File}
	switch {
	case group == "script" && a.File != "":
		sp.Panel = "code"
		if a.Old != "" {
			sp.Line = s.Nav.ScriptLine(a.File, a.Old)
		}
		sp.Line = max(sp.Line, 1)
		return sp, true
	case tool == "scene_run":
		sp.Panel = "console"
		return sp, true
	case strings.HasSuffix(tool, "_validate") || tool == "refs_missing":
		sp.Panel = "problems"
		return sp, true
	case strings.HasPrefix(tool, "data_") && a.File != "":
		sp.Panel, sp.Field = "table", a.Row+a.ID
		return sp, true
	case !scene(a.File):
		return sp, false
	}
	addr := a.Node
	if addr == "" {
		addr = a.Parent
	}
	sp.Type = comp(a)
	if open {
		if _, ok := s.Nav.OpenScene(a.File); !ok {
			return sp, false
		}
	}
	if addr != "" {
		if id, ok := s.Nav.Node(a.File, addr); ok {
			sp.Node = id
		}
	}
	return sp, true
}

// GoTo takes the editor to a place: the scene, the node selected and framed, the component; the script line; the panel.
func (s *Service) GoTo(sp presence.Spot) {
	switch sp.Panel {
	case "code":
		s.Nav.OpenScript(sp.Doc, sp.Line)
	case "console":
		s.Nav.ShowConsole()
	case "problems":
		s.Nav.ShowProblems()
	case "table":
		s.Nav.OpenData(sp.Doc, sp.Field)
	default:
		if sp.Doc == "" {
			return
		}
		if _, ok := s.Nav.OpenScene(sp.Doc); !ok || sp.Node.IsZero() {
			return
		}
		s.selectNode(sp.Doc, sp.Node)
		s.Nav.Reveal(sp.Doc, sp.Node)
		s.Nav.Frame(sp.Doc, sp.Node)
		s.Nav.Inspect(sp.Doc, sp.Node, sp.Type)
	}
}

// comp is the component a call names: its comp argument, or the first part of a path such as Motion2D.maxSpeed.
func comp(a args) string {
	if a.Comp != "" {
		return a.Comp
	}
	if i := strings.LastIndexByte(a.Path, '.'); i > 0 && !strings.HasPrefix(a.Path, "transform.") {
		return a.Path[:i]
	}
	return ""
}

func (s *Service) selectNode(rel string, node ids.NodeID) {
	if s.Sel == nil {
		return
	}
	if doc, ok := s.Nav.OpenScene(rel); ok {
		s.Sel.Set(selection.Item{Doc: doc, Node: node})
	}
}

// Cards follows the cards: a waiting one marks the fields and lines it will change, an applied one lights them and takes the editor to them.
func (s *Service) Cards(cards []agentui.Card, now float64) {
	s.edits = s.edits[:0]
	for i := range cards {
		c := &cards[i]
		prev, seen := s.seen[c.ID]
		if !seen || prev != c.State {
			s.seen[c.ID] = c.State
			s.changed(c, prev, seen, now)
		}
		if c.State == agentui.Pending || c.State == agentui.Approved {
			for _, ch := range c.Changes {
				if !ch.NodeID.IsZero() {
					s.edits = append(s.edits, presence.Edit{Spot: s.spotOf(c.File, &ch), Card: c.ID})
				}
			}
		}
	}
	s.Model.SetEdits(s.edits)
}

func (s *Service) spotOf(file string, ch *agentui.Change) presence.Spot {
	sp := presence.Spot{Doc: file, Node: ch.NodeID}
	if ch.Type != 0 {
		sp.Type = s.Nav.TypeName(ch.Type)
		if ch.Field != 0 {
			sp.Field = s.Nav.FieldName(ch.Type, ch.Field)
		}
	}
	return sp
}

func (s *Service) changed(c *agentui.Card, prev agentui.State, seen bool, now float64) {
	switch c.State {
	case agentui.Pending:
		for _, ch := range c.Changes {
			switch {
			case ch.Op == "script-edit" || ch.Op == "script-new":
				s.Model.Mark(presence.Lines{Doc: ch.Node, From: max(ch.Line, 1), To: max(ch.Line, 1) + lineCount(ch.Before) - 1, At: now, Pending: strings.Split(strings.TrimRight(ch.After, "\n"), "\n")})
			case ch.Op == "remove-node" && !ch.NodeID.IsZero():
				if box, ok := s.Nav.Bounds(c.File, ch.NodeID); ok {
					s.ghosts = append(s.ghosts, ghost{card: c.ID, flash: presence.Flash{Spot: presence.Spot{Doc: c.File, Node: ch.NodeID}, Kind: presence.Removed, Ghost: box, Name: strings.TrimPrefix(ch.Node, "Root/")}})
				}
			}
		}
	case agentui.Applied:
		s.applied(c, now)
	case agentui.Rejected, agentui.Expired, agentui.Failed, agentui.Undone:
		for _, ch := range c.Changes {
			if ch.Op == "script-edit" || ch.Op == "script-new" {
				s.Model.Unmark(ch.Node, max(ch.Line, 1))
			}
		}
		s.dropGhosts(c.ID)
	}
}

func lineCount(text string) int {
	if text == "" {
		return 1
	}
	return strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
}

func (s *Service) applied(c *agentui.Card, now float64) {
	changes := c.Applied
	if len(changes) == 0 {
		changes = c.Changes
	}
	var first ids.NodeID
	for _, ch := range changes {
		switch ch.Op {
		case "script-edit", "script-new":
			line := max(ch.Line, 1)
			s.Model.Unmark(ch.Node, line)
			s.Model.Mark(presence.Lines{Doc: ch.Node, From: line, To: line + lineCount(ch.After) - 1, At: now})
			sp := presence.Spot{Doc: ch.Node, Line: line, Panel: "code"}
			s.Model.Move(Agent, sp, now)
			if s.Active() {
				s.GoTo(sp)
			}
			continue
		}
		if ch.NodeID.IsZero() {
			continue
		}
		sp := s.spotOf(c.File, &ch)
		kind := presence.Changed
		switch ch.Op {
		case "add-node", "prefab-instantiate":
			kind, sp.Type, sp.Field = presence.Added, "", ""
		case "add-comp":
			kind, sp.Field = presence.Added, ""
		}
		s.Model.Light(presence.Flash{Spot: sp, Kind: kind, At: now})
		if first.IsZero() {
			first = ch.NodeID
		}
		s.redo(sp)
	}
	for _, g := range s.ghosts {
		if g.card == c.ID {
			g.flash.At = now
			s.Model.Light(g.flash)
		}
	}
	s.dropGhosts(c.ID)
	if !first.IsZero() && c.File != "" {
		sp := presence.Spot{Doc: c.File, Node: first}
		s.Model.Move(Agent, sp, now)
		if s.Active() {
			s.GoTo(sp)
		}
	}
}

// Defer keeps the user's edit of a field a waiting card will change, to make it again after the card.
func (s *Service) Defer(sp presence.Spot, run func()) {
	for i := range s.deferred {
		if s.deferred[i].spot == sp {
			s.deferred[i].run = run
			return
		}
	}
	s.deferred = append(s.deferred, deferred{spot: sp, run: run})
}

// redo makes the user's deferred edit of a field again, as its own step after the agent's.
func (s *Service) redo(sp presence.Spot) {
	for i := 0; i < len(s.deferred); i++ {
		d := s.deferred[i]
		if d.spot.Doc == sp.Doc && d.spot.Node == sp.Node && d.spot.Type == sp.Type && (sp.Field == "" || d.spot.Field == sp.Field) {
			s.deferred = append(s.deferred[:i], s.deferred[i+1:]...)
			i--
			d.run()
		}
	}
}

func (s *Service) dropGhosts(card int) {
	n := 0
	for _, g := range s.ghosts {
		if g.card != card {
			s.ghosts[n] = g
			n++
		}
	}
	s.ghosts = s.ghosts[:n]
}
