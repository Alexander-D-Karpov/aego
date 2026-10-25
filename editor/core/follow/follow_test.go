package follow

import (
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/presence"
	"aego/editor/core/selection"
	"aego/schema"
)

type nav struct {
	opened, revealed, framed, script string
	line                             int
	problems                         bool
}

var player = ids.NodeID{Hi: 1, Lo: 7}

func (n *nav) OpenScene(rel string) (selection.DocID, bool)     { n.opened = rel; return 3, true }
func (n *nav) Node(rel, addr string) (ids.NodeID, bool)         { return player, addr == "Player" }
func (n *nav) Reveal(rel string, node ids.NodeID)               { n.revealed = rel }
func (n *nav) Frame(rel string, node ids.NodeID)                { n.framed = rel }
func (n *nav) Inspect(rel string, node ids.NodeID, comp string) {}
func (n *nav) OpenScript(rel string, line int)                  { n.script, n.line = rel, line }
func (n *nav) ScriptLine(rel, text string) int                  { return 12 }
func (n *nav) Bounds(string, ids.NodeID) (mathx.Rect, bool)     { return mathx.Rect{W: 1, H: 1}, true }
func (n *nav) ShowProblems()                                    { n.problems = true }
func (n *nav) ShowConsole()                                     {}
func (n *nav) OpenData(rel, row string)                         {}
func (n *nav) TypeName(schema.TypeID) string                    { return "gameplay.Motion2D" }
func (n *nav) FieldName(schema.TypeID, schema.FieldID) string   { return "maxSpeed" }

func TestFollowLeadsToTheNodeAndPausesForTheUser(t *testing.T) {
	n := &nav{}
	sel := selection.New()
	s := New(presence.New(), sel, n)
	s.SetOn(true)
	s.Step("scene_set", `{"file":"scenes/main.aescene","node":"Player","path":"gameplay.Motion2D.maxSpeed"}`, 1)
	if n.opened != "scenes/main.aescene" || n.revealed == "" || n.framed == "" {
		t.Fatalf("nav = %+v", n)
	}
	if it, ok := sel.Active(); !ok || it.Node != player {
		t.Fatal("the agent's node is not in the agent selection")
	}
	if sp, _, ok := s.Model.Where(Agent, 2); !ok || sp.Node != player || sp.Type != "gameplay.Motion2D" {
		t.Fatalf("presence = %+v", sp)
	}
	s.UserActive(3)
	*n = nav{}
	s.Step("scene_get", `{"file":"scenes/main.aescene","node":"Player"}`, 4)
	if n.opened != "" || !s.Paused() {
		t.Fatal("following went on while the user worked")
	}
	s.Tick(3 + DefaultResume)
	if s.Paused() {
		t.Fatal("following did not resume after the user was idle")
	}
	s.Step("script_edit", `{"file":"scripts/enemy/enemy.aes","old":"x := 1"}`, 40)
	if n.script != "scripts/enemy/enemy.aes" || n.line != 12 {
		t.Fatalf("script = %q:%d", n.script, n.line)
	}
}

func TestFollowLightsAppliedChangesAndKeepsTheUsersEdit(t *testing.T) {
	s := New(presence.New(), selection.New(), &nav{})
	s.SetOn(true)
	card := agentui.Card{ID: 1, File: "scenes/main.aescene", State: agentui.Pending, Changes: []agentui.Change{{Op: "set", NodeID: player, Type: schema.TypeID(5), Field: schema.FieldID(2)}}}
	s.Cards([]agentui.Card{card}, 1)
	if !s.Model.Editing("scenes/main.aescene", player, "gameplay.Motion2D", "maxSpeed") {
		t.Fatal("a waiting card does not mark the field it changes")
	}
	redone := false
	s.Defer(presence.Spot{Doc: "scenes/main.aescene", Node: player, Type: "gameplay.Motion2D", Field: "maxSpeed"}, func() { redone = true })
	card.State = agentui.Applied
	s.Cards([]agentui.Card{card}, 2)
	if a := s.Model.FieldLight("scenes/main.aescene", player, "gameplay.Motion2D", "maxSpeed", 2.5); a <= 0 {
		t.Fatal("the changed field is not lit")
	}
	if !redone {
		t.Fatal("the user's edit of the same field was not made again after the card")
	}
	if s.Model.Editing("scenes/main.aescene", player, "gameplay.Motion2D", "maxSpeed") {
		t.Fatal("an applied card still marks its field")
	}
}
