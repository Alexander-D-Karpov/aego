//go:build aego_golden

package golden

import (
	"errors"
	"testing"
	"time"

	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/context"
	"aego/editor/core/services"
	"aego/editor/panels/agent"
	edocs "aego/editor/panels/docs"
	"aego/ui/dnd"
)

type goldenHost struct {
	missing error
	events  []agentui.Event
	cards   []agentui.Card
	auto    bool
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
func (h *goldenHost) Busy() bool          { return false }
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

func agentGolden(t *testing.T, name string, h *goldenHost) {
	t.Helper()
	env := newEditorEnvSize(t, 420, 520)
	ctx := &context.Editor{}
	p := agent.New(agent.Deps{Host: h, Docs: ctx, Notices: ctx, Root: t.TempDir(), Highlight: edocs.Highlight})
	deadline := time.Now().Add(5 * time.Second)
	for !p.Checked() && time.Now().Before(deadline) {
		p.Tick()
		time.Sleep(time.Millisecond)
	}
	p.Tick()
	for range 3 {
		img, err := env.draw(func(full mathx.Rect) {
			p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 1}, full)
		})
		if err != nil {
			t.Fatal(err)
		}
		_ = img
	}
	img, err := env.draw(func(full mathx.Rect) {
		p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 1}, full)
	})
	checkGolden(t, name, img, err)
}

var platformCard = agentui.Card{ID: 1, Command: "scene.add-node", State: agentui.Pending, Changes: []agentui.Change{
	{Op: "add-node", Node: "Root/Ledge"},
	{Op: "set", Node: "Root/Ledge", Path: "transform.position", Before: "[0, 0]", After: "[-13, 1.5]"},
}}

func TestEditorAgentPanelMissing(t *testing.T) {
	agentGolden(t, "editor_agent_missing", &goldenHost{missing: errors.New(`exec: "claude": executable file not found in $PATH`)})
}

func TestEditorAgentPanelCard(t *testing.T) {
	agentGolden(t, "editor_agent_card", &goldenHost{events: []agentui.Event{
		{Kind: agentui.EvText, Text: "The player stands on **GroundLeft**. I will add a ledge above it:\n\n- a node `Ledge`\n- at `[-13, 1.5]`"},
		{Kind: agentui.EvToolStart, Tool: "scene_tree", CallID: "1", Args: `{"file":"scenes/main.aescene"}`},
		{Kind: agentui.EvToolResult, Tool: "scene_tree", CallID: "1", Result: "ok"},
		{Kind: agentui.EvToolStart, Tool: "scene_add_node", CallID: "2", Args: `{"file":"scenes/main.aescene","name":"Ledge"}`},
	}, cards: []agentui.Card{platformCard}})
}

func TestEditorAgentPanelAutoApply(t *testing.T) {
	applied := platformCard
	applied.State, applied.CanUndo = agentui.Applied, true
	removal := agentui.Card{ID: 2, Command: "scene.remove-node", State: agentui.Pending, Destructive: true, Changes: []agentui.Change{{Op: "remove-node", Node: "Root/Crate"}}}
	agentGolden(t, "editor_agent_auto", &goldenHost{auto: true, events: []agentui.Event{
		{Kind: agentui.EvText, Text: "Added the ledge. Removing the crate needs your answer."},
	}, cards: []agentui.Card{applied, removal}})
}
