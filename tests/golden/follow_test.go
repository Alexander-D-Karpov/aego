//go:build aego_golden

package golden

import (
	"image"
	"testing"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/agentui"
	"aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/follow"
	"aego/editor/core/presence"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/editor/ext/inspect"
	"aego/editor/panels/hierarchy"
	"aego/editor/panels/inspector"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
	"aego/ui/dnd"
)

// The Hierarchy shows the agent's marker on the node it works on, its selection outline and the light of a node it added (11b5b).
func TestEditorHierarchyAgent(t *testing.T) {
	env := newEditorEnvSize(t, 320, 240)
	ctx, s := goldenScene(t, env)
	d := ctx.Doc()
	player := s.ID(s.Find(s.Root(), "Player"))
	ledge := s.ID(s.Find(s.Root(), "Ledge"))
	m := presence.New()
	m.Move(follow.Agent, presence.Spot{Doc: d.Path, Node: player}, 1)
	m.Light(presence.Flash{Spot: presence.Spot{Doc: d.Path, Node: ledge}, Kind: presence.Added, At: 1})
	sel := selection.New()
	sel.Set(selection.Item{Doc: d.ID, Node: player})
	ctx.Select(s.ID(s.Find(s.Root(), "Ground")))
	p := hierarchy.New(hierarchy.Deps{Docs: ctx, Selection: ctx, Commands: ctx.Commands, Notices: ctx, Tools: &ctx.Settings, Presence: m, AgentSel: sel})
	var img *image.NRGBA
	var err error
	for range 3 {
		img, err = env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}, Now: 1.5}, full) })
	}
	checkGolden(t, "editor_hierarchy_agent", img, err)
}

// The Agent panel while following: the row under the header says where the agent is, the dots above the input are its steps.
func TestEditorAgentPanelFollowing(t *testing.T) {
	h := cardHost()
	applied := platformCard
	applied.State = agentui.Applied
	h.cards = []agentui.Card{applied}
	h.events = append(h.events,
		agentui.Event{Kind: agentui.EvToolResult, Tool: "scene_apply", CallID: "2", Result: `{"ok":true}`},
		agentui.Event{Kind: agentui.EvToolStart, Tool: "scene_validate", CallID: "3", Args: `{"file":"scenes/main.aescene"}`},
		agentui.Event{Kind: agentui.EvToolResult, Tool: "scene_validate", CallID: "3", Failed: true, Result: "node Ledge has no collider"},
	)
	h.follow = true
	agentGolden(t, "editor_agent_following", h)
}

// goldenScene is a small scene open in an editor context: Ground, Player with a Sprite, and Ledge.
func goldenScene(t *testing.T, env *editorEnv) (*context.Editor, *scene.Scene) {
	t.Helper()
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	s.Create(s.Root(), "Ground")
	player := s.Create(s.Root(), "Player")
	s.Create(player, "Sprite")
	s.Create(s.Root(), "Ledge")
	s.Flush()
	ctx := &context.Editor{
		UI:        env.ui,
		Types:     types,
		Docs:      document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()}),
		Selection: selection.New(),
		Commands:  command.New(),
		DnD:       &dnd.State{},
		Settings:  context.DefaultSettings(),
		Collapsed: map[string]bool{},
	}
	ctx.Docs.New("scenes/main.aescene", s)
	ctx.Doc().View.Expanded[s.ID(player)] = true
	return ctx, s
}

// nopNav is follow mode with no editor behind it, for the panel's screenshots.
type nopNav struct{}

func (nopNav) OpenScene(string) (selection.DocID, bool)       { return 0, false }
func (nopNav) Node(string, string) (ids.NodeID, bool)         { return ids.NodeID{}, false }
func (nopNav) Reveal(string, ids.NodeID)                      {}
func (nopNav) Frame(string, ids.NodeID)                       {}
func (nopNav) Inspect(string, ids.NodeID, string)             {}
func (nopNav) OpenScript(string, int)                         {}
func (nopNav) ScriptLine(string, string) int                  { return 0 }
func (nopNav) Bounds(string, ids.NodeID) (mathx.Rect, bool)   { return mathx.Rect{}, false }
func (nopNav) ShowProblems()                                  {}
func (nopNav) ShowConsole()                                   {}
func (nopNav) OpenData(string, string)                        {}
func (nopNav) TypeName(schema.TypeID) string                  { return "" }
func (nopNav) FieldName(schema.TypeID, schema.FieldID) string { return "" }

// The Inspector while following shows the agent's node: a changed field lit, a warning on a field a waiting card will change.
func TestEditorInspectorAgent(t *testing.T) {
	env := newEditorEnvSize(t, 420, 480)
	types, err := assembly.NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builtin.Default().Load(types, bind.NewManifest(), assembly.Options{}, goldenModule{}); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	holder := s.Create(s.Root(), "Holder")
	target := s.Create(s.Root(), "Target")
	scene.Add[goldenRefs](s, holder).Alive = scene.NewNodeRef(s.ID(target))
	s.Flush()
	ctx := &context.Editor{
		UI: env.ui, Types: types, Selection: selection.New(), Commands: command.New(), DnD: &dnd.State{},
		Docs:     document.NewManager(document.Options{Root: t.TempDir(), Types: types, Log: log.Nop()}),
		Settings: context.DefaultSettings(), Collapsed: map[string]bool{},
	}
	ctx.Docs.New("scenes/refs.aescene", s)
	d := ctx.Doc()
	m := presence.New()
	m.Following = true
	id := s.ID(holder)
	m.Light(presence.Flash{Spot: presence.Spot{Doc: d.Path, Node: id, Type: "golden.Refs", Field: "alive"}, At: 1})
	m.SetEdits([]presence.Edit{{Spot: presence.Spot{Doc: d.Path, Node: id, Type: "golden.Refs", Field: "noCam"}}})
	sel := selection.New()
	sel.Set(selection.Item{Doc: d.ID, Node: id})
	reg := inspect.NewRegistry()
	if err := reg.Load(types.Schema()); err != nil {
		t.Fatal(err)
	}
	reg.SetEditor(ctx)
	p := inspector.New(reg, inspector.Deps{Docs: ctx, Selection: ctx, Commands: ctx.Commands, Notices: ctx, Nav: ctx, Problems: ctx,
		Clip: clipboard.EnvOf(ctx), Collapsed: ctx.Collapsed, Presence: m, AgentSel: sel})
	var img *image.NRGBA
	for range 3 {
		img, err = env.draw(func(full mathx.Rect) { p.Draw(&services.Frame{UI: env.ui, DnD: ctx.DnD, Now: 1.4}, full) })
	}
	checkGolden(t, "editor_inspector_agent", img, err)
}
