package shell

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	agentpanel "aego/editor/panels/agent"
	"aego/ui/dock"
)

type defaultLayoutPanel string

func (p defaultLayoutPanel) ID() string                       { return string(p) }
func (p defaultLayoutPanel) Title() string                    { return string(p) }
func (p defaultLayoutPanel) Draw(*services.Frame, mathx.Rect) {}

func TestDesignDefaultAgentAndSavedLayouts(t *testing.T) {
	root := t.TempDir()
	makeWorkspace := func(withAgent bool) *workspace.Workspace {
		w := workspace.New(root, nil)
		ids := []string{workspace.Hierarchy, workspace.Assets, workspace.Scene, workspace.Game, workspace.Inspector, workspace.Console, workspace.Problems}
		if withAgent {
			ids = append(ids, agentpanel.ID)
		}
		for i, id := range ids {
			if err := w.Register(workspace.Desc{Panel: defaultLayoutPanel(id), Order: i}); err != nil {
				t.Fatal(err)
			}
		}
		for _, p := range BuiltinWorkspaces() {
			if err := w.RegisterPreset(p); err != nil {
				t.Fatal(err)
			}
		}
		w.LoadLayout()
		return w
	}
	w := makeWorkspace(true)
	a, ok := w.Tree.Node(w.Tree.FindPanel(agentpanel.ID))
	if !ok {
		t.Fatal("Agent is not open in Design")
	}
	i, ok := w.Tree.Node(w.Tree.FindPanel(workspace.Inspector))
	if !ok || a.Parent != i.Parent {
		t.Fatal("Agent and Inspector are not in the same split")
	}
	split, ok := w.Tree.Node(a.Parent)
	if !ok || split.Kind != dock.KindSplit || split.Axis != dock.AxisY || split.First != i.ID || split.Second != a.ID || split.Ratio != 0.55 {
		t.Fatalf("Agent must be below Inspector: %+v", split)
	}
	if active, _ := a.ActivePanel(); active != agentpanel.ID {
		t.Fatalf("Agent is not the visible tab: %q", active)
	}
	w.Tree.Undock(agentpanel.ID)
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	if makeWorkspace(true).Tree.Has(agentpanel.ID) {
		t.Fatal("the new default replaced a saved user layout")
	}
	// No optional Agent panel must still produce a valid default when reset.
	without := makeWorkspace(false)
	without.ResetLayout()
	if !without.Tree.Has(workspace.Inspector) || without.Tree.Has(agentpanel.ID) {
		t.Fatal("missing optional Agent broke the default layout")
	}
}
