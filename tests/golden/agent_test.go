//go:build aego_golden

package golden

import (
	"encoding/json"
	"errors"
	"image"
	"testing"
	"time"

	"aego/app/agent"
	"aego/core/ids"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/services"
	agentpanel "aego/editor/panels/agent"
	edocs "aego/editor/panels/docs"
	"aego/ui/dnd"
)

type goldenHost struct {
	missing   error
	events    []agentui.Event
	cards     []agentui.Card
	questions []agentui.Question
	details   map[string]agentui.Detail
	auto      bool
	plan      bool
	busy      bool
	expand    bool
	user      string
}

func (h *goldenHost) Available(string) (string, error) {
	if h.missing != nil {
		return "", h.missing
	}
	return "2.1.296 (Claude Code)", nil
}
func (h *goldenHost) Command() string     { return "claude" }
func (h *goldenHost) SetCommand(string)   {}
func (h *goldenHost) Running() bool       { return false }
func (h *goldenHost) Busy() bool          { return h.busy }
func (h *goldenHost) Send(string) error   { return nil }
func (h *goldenHost) Cancel()             {}
func (h *goldenHost) Close()              {}
func (h *goldenHost) Decide(int, bool)    {}
func (h *goldenHost) Undo(int) error      { return nil }
func (h *goldenHost) AutoApply() bool     { return h.auto }
func (h *goldenHost) SetAutoApply(v bool) { h.auto = v }
func (h *goldenHost) Poll(dst []agentui.Event) []agentui.Event {
	dst = append(dst, h.events...)
	h.events = nil
	return dst
}
func (h *goldenHost) Cards(dst []agentui.Card) []agentui.Card { return append(dst, h.cards...) }
func (h *goldenHost) PlanOnly() bool                          { return h.plan }
func (h *goldenHost) SetPlanOnly(v bool)                      { h.plan = v }
func (h *goldenHost) Reset()                                  {}
func (h *goldenHost) Answer(int, []int, string)               {}
func (h *goldenHost) Questions(dst []agentui.Question) []agentui.Question {
	return append(dst, h.questions...)
}

// Describe uses the real look of each tool, so the goldens show the titles the editor shows.
func (h *goldenHost) Describe(tool, args, result string, failed bool) agentui.Step {
	var a agent.Args
	_ = json.Unmarshal([]byte(args), &a)
	st := agentui.Step{Icon: "code.block", Title: tool}
	if l, ok := agent.Present(tool); ok {
		st.Icon, st.Title = l.Icon, l.Title(a)
		if out, ok := agent.DecodeOutput(result); ok && !failed {
			st.Summary = l.Summary(out)
		}
	}
	for _, c := range agent.Commands() {
		if c.Tool() == tool {
			st.Write = c.Write
		}
	}
	if failed {
		st.Summary = result
		st.Detail = agentui.Detail{Kind: agentui.DetailText, Text: result}
	}
	if d, ok := h.details[tool]; ok && result != "" {
		st.Detail = d
	}
	return st
}

func agentGolden(t *testing.T, name string, h *goldenHost) {
	t.Helper()
	img, err := shotAgent(t, newEditorEnvSize(t, 420, 520), h)
	checkGolden(t, name, img, err)
}

func shotAgent(t *testing.T, env *editorEnv, h *goldenHost) (*image.NRGBA, error) {
	t.Helper()
	ctx := &context.Editor{}
	at := time.Date(2026, 10, 26, 14, 30, 0, 0, time.UTC)
	p := agentpanel.New(agentpanel.Deps{Host: h, Docs: ctx, Notices: ctx, Root: t.TempDir(), Highlight: edocs.Highlight, Now: func() time.Time { return at }})
	if h.user != "" {
		p.Submit(h.user)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !p.Checked() && time.Now().Before(deadline) {
		p.Tick()
		time.Sleep(time.Millisecond)
	}
	p.Tick()
	if h.expand {
		p.Expand()
	}
	for range 3 {
		img, err := env.draw(func(full mathx.Rect) {
			p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 1}, full)
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = img
	}
	return env.draw(func(full mathx.Rect) {
		p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 1}, full)
	})
}

var platformCard = agentui.Card{ID: 1, Command: "scene.apply", File: "scenes/main.aescene", Args: `{"file":"scenes/main.aescene","tx":"tx version=1 { label=\"Add Ledge\" }"}`, State: agentui.Pending,
	Changes: []agentui.Change{
		{Op: "add-node", Node: "Root/Ledge"},
		{Op: "set", Node: "Root/Ledge", Path: "transform.position", Before: "[0, 0]", After: "[-13, 1.5]"},
	},
	Rows: []agentui.Row{
		{Group: "Ledge", Title: "Ledge", Icon: "edit.add", Text: "new node"},
		{Group: "Ledge", Title: "Ledge", Icon: "edit.add", Text: "+ Sprite Renderer"},
		{Group: "Ledge", Title: "Ledge", Icon: "edit.set", Text: "Transform › Position", Before: "[0, 0]", After: "[-13, 1.5]"},
		{Group: "Ledge", Title: "Ledge", Icon: "edit.set", Text: "Sprite Renderer › Sprite", Before: "none", After: "platform.png"},
		{Group: "Player", Title: "Player", Icon: "edit.set", Text: "Motion 2D › Max Speed", Before: "7", After: "9"},
	}}

func TestEditorAgentPanelMissing(t *testing.T) {
	agentGolden(t, "editor_agent_missing", &goldenHost{missing: errors.New(`exec: "claude": executable file not found in $PATH`)})
}

func TestEditorAgentPanelCard(t *testing.T) {
	agentGolden(t, "editor_agent_card", cardHost())
}

func cardHost() *goldenHost {
	return &goldenHost{events: []agentui.Event{
		{Kind: agentui.EvText, Text: "The player stands on **GroundLeft**. I will add a ledge above it:\n\n- a node `Ledge`\n- at `[-13, 1.5]`"},
		{Kind: agentui.EvToolStart, Tool: "scene_tree", CallID: "1", Args: `{"file":"scenes/main.aescene"}`},
		{Kind: agentui.EvToolResult, Tool: "scene_tree", CallID: "1", Result: "ok"},
		{Kind: agentui.EvToolStart, Tool: "scene_apply", CallID: "2", Args: `{"file":"scenes/main.aescene","tx":"tx version=1 { label=\"Add Ledge\" }"}`},
	}, cards: []agentui.Card{platformCard}}
}

func TestEditorAgentPanelAutoApply(t *testing.T) {
	applied := platformCard
	applied.State, applied.CanUndo = agentui.Applied, true
	removal := agentui.Card{ID: 2, Command: "scene.remove-node", Args: `{"file":"scenes/main.aescene","node":"Crate"}`, State: agentui.Pending, Destructive: true,
		Changes: []agentui.Change{{Op: "remove-node", Node: "Root/Crate"}}, Rows: []agentui.Row{{Group: "Crate", Title: "Crate", Icon: "edit.remove", Text: "removed"}}}
	agentGolden(t, "editor_agent_auto", &goldenHost{auto: true, events: []agentui.Event{
		{Kind: agentui.EvText, Text: "Added the ledge. Removing the crate needs your answer."},
	}, cards: []agentui.Card{applied, removal}})
}

// A message whose read-only steps fold into one row, with one step open: the docs search lists what it found.
func TestEditorAgentPanelSteps(t *testing.T) {
	result := func(v any) string {
		b, _ := json.Marshal(map[string]any{"ok": true, "value": v})
		return string(b)
	}
	agentGolden(t, "editor_agent_steps", &goldenHost{expand: true, user: "How does @Coin tell the game it was taken?", details: map[string]agentui.Detail{
		"docs_search": {Kind: agentui.DetailList, Rows: []agentui.Row{
			{Icon: "cat.node", Title: "Pickup 2D", Text: "An area that a body with the right tag collects.", Link: "doc:gameplay.Pickup2D"},
			{Icon: "code.block", Title: "Collected", Text: "Fires when a body collects the pickup.", Link: "doc:gameplay.Pickup2D.Collected"},
		}},
		"scene_tree": {Kind: agentui.DetailTree, Rows: []agentui.Row{
			{Icon: "cat.node", Title: "Player", Text: "Sprite Renderer, Rigid Body 2D, Motion 2D"},
			{Icon: "cat.node", Title: "Shape", Text: "Capsule Collider 2D", Depth: 1},
			{Icon: "file.prefab", Title: "EnemyLeft", Text: "Health, Hitbox 2D"},
		}},
	}, events: []agentui.Event{
		{Kind: agentui.EvToolStart, Tool: "scene_tree", CallID: "1", Args: `{"file":"scenes/main.aescene"}`},
		{Kind: agentui.EvToolResult, Tool: "scene_tree", CallID: "1", Result: result(map[string]any{"name": "Root", "children": []any{map[string]any{"name": "Player"}, map[string]any{"name": "EnemyLeft"}}})},
		{Kind: agentui.EvToolStart, Tool: "docs_search", CallID: "2", Args: `{"query":"Pickup2D Collected"}`},
		{Kind: agentui.EvToolResult, Tool: "docs_search", CallID: "2", Result: `{"results":[{"id":"gameplay.Pickup2D"},{"id":"gameplay.Pickup2D.Collected"}]}`},
		{Kind: agentui.EvToolStart, Tool: "schema_show", CallID: "3", Args: `{"type":"gameplay.Pickup2D"}`},
		{Kind: agentui.EvToolResult, Tool: "schema_show", CallID: "3", Result: result(map[string]any{"name": "gameplay.Pickup2D"})},
		{Kind: agentui.EvText, Text: "Pickup 2D fires **Collected** when the player takes it; a script on the coin can count it."},
	}})
}

func TestEditorAgentPanelApplied(t *testing.T) {
	applied := platformCard
	applied.State, applied.CanUndo = agentui.Applied, true
	applied.Applied = []agentui.Change{{Op: "add-node", Node: "Root/Ledge", NodeID: ids.NodeID{Hi: 1, Lo: 2}}}
	h := cardHost()
	h.cards, h.expand = []agentui.Card{applied}, true
	agentGolden(t, "editor_agent_applied", h)
}

func TestEditorAgentPanelError(t *testing.T) {
	agentGolden(t, "editor_agent_error", &goldenHost{events: []agentui.Event{
		{Kind: agentui.EvToolStart, Tool: "scene_run", CallID: "1", Args: `{"file":"scenes/main.aescene","frames":240}`},
		{Kind: agentui.EvToolResult, Tool: "scene_run", CallID: "1", Failed: true, Result: "scenes/main.aescene: node Finish has Pickup 2D without a collider"},
		{Kind: agentui.EvText, Text: "The run failed; the Finish node needs a collider."},
	}})
}

func TestEditorAgentPanelEmpty(t *testing.T) {
	agentGolden(t, "editor_agent_empty", &goldenHost{})
}

func TestEditorAgentPanelQuestion(t *testing.T) {
	agentGolden(t, "editor_agent_question", questionHost())
}

func questionHost() *goldenHost {
	return &goldenHost{busy: true, events: []agentui.Event{
		{Kind: agentui.EvText, Text: "Scenes cannot load each other yet, so there are three ways to make the levels."},
	}, questions: []agentui.Question{{ID: 7, State: agentui.Pending, Other: true, Text: "How should the four levels be made?", Options: []agentui.Option{
		{Label: "All in main.aescene, a teleport between them", Detail: "Works now; the levels share one scene"},
		{Label: "Separate scenes and a level exit", Detail: "Needs scene loading in the engine first"},
		{Label: "Three scenes now, exits later", Detail: "Geometry, enemies and cameras without transitions"},
	}}}}
}
