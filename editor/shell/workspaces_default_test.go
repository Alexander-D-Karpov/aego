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

type scopedLayoutPanel struct {
	defaultLayoutPanel
	scope string
}

func (p scopedLayoutPanel) Scope() string { return p.scope }

func TestArtFactoryToolsAndLayersAreSeparateWithoutResettingOverrides(t *testing.T) {
	root := t.TempDir()
	makeWorkspace := func() *workspace.Workspace {
		w := workspace.New(root, nil)
		for _, id := range []string{workspace.Assets, workspace.Hierarchy, workspace.Scene, workspace.Inspector, "art.tools", "art.properties", "art.timeline", "art.preview"} {
			if err := w.Register(workspace.Desc{Panel: defaultLayoutPanel(id)}); err != nil {
				t.Fatal(err)
			}
		}
		for _, p := range BuiltinWorkspaces() {
			if err := w.RegisterPreset(p); err != nil {
				t.Fatal(err)
			}
		}
		w.LoadLayout()
		w.Switch(workspace.Art)
		return w
	}
	w := makeWorkspace()
	if !w.Tree.Has("art.tools") || !w.Tree.Has("art.properties") || !w.Tree.Has("art.timeline") || w.Tree.Has(workspace.Hierarchy) {
		t.Fatal("Art factory composition is not the dedicated authoring layout")
	}
	tools, layers := w.Tree.FindPanel("art.tools"), w.Tree.FindPanel("art.properties")
	if tools == 0 || layers == 0 || tools == layers {
		t.Fatal("Tools and Palette/Layers share a tab group")
	}
	w.Tree.Undock("art.tools")
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	if makeWorkspace().Tree.Has("art.tools") {
		t.Fatal("factory update overwrote a saved layout")
	}
	w.ResetLayout()
	if !w.Tree.Has("art.tools") {
		t.Fatal("Reset Workspace did not restore factory Tools")
	}
}

func TestAnimationFactoryShowsFocusedCanvasBesidePreview(t *testing.T) {
	w := workspace.New(t.TempDir(), nil)
	for _, id := range []string{workspace.Scene, "art.tools", "art.properties", "art.timeline", "art.preview"} {
		if err := w.Register(workspace.Desc{Panel: defaultLayoutPanel(id)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"first", "second"} {
		if err := w.Register(workspace.Desc{Panel: scopedLayoutPanel{defaultLayoutPanel("render2d.art/" + id), "render2d.art"}, Slot: workspace.SlotCenter}); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range BuiltinWorkspaces() {
		if err := w.RegisterPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	w.LoadLayout()
	w.Focus("render2d.art/first")
	w.Switch(workspace.Animation)
	canvas, ok := w.Tree.Node(w.Tree.FindPanel("render2d.art/first"))
	if !ok {
		t.Fatal("focused art document is absent")
	}
	active, _ := canvas.ActivePanel()
	if active != "render2d.art/first" || canvas.ID == w.Tree.FindPanel("art.preview") {
		t.Fatal("factory Animation did not show focused canvas beside preview", active)
	}
	w.Focus(workspace.Scene)
	w.Switch(workspace.Design)
	w.Switch(workspace.Animation)
	canvas, _ = w.Tree.Node(w.Tree.FindPanel(workspace.Scene))
	if active, _ := canvas.ActivePanel(); active != workspace.Scene {
		t.Fatal("factory preference replaced a user's saved active tab", active)
	}
}

func TestVFXKeepsCanvasAndParticlePresetVisible(t *testing.T) {
	w := workspace.New(t.TempDir(), nil)
	for i, id := range []string{workspace.Assets, workspace.Scene, workspace.Inspector, "art.properties", "art.timeline", "art.preview"} {
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
	if !w.Switch(workspace.VFX) {
		t.Fatal("VFX workspace unavailable")
	}
	const canvas, effect = "render2d.art/test", "particles.preset/test"
	for _, panel := range []scopedLayoutPanel{{defaultLayoutPanel(canvas), "render2d.art"}, {defaultLayoutPanel(effect), "particles.preset"}} {
		if err := w.Register(workspace.Desc{Panel: panel, Slot: workspace.SlotCenter}); err != nil {
			t.Fatal(err)
		}
		w.Focus(panel.ID())
	}
	canvasNode := w.Tree.FindPanel(canvas)
	effectNode := w.Tree.FindPanel(effect)
	if canvasNode == 0 || effectNode == 0 || canvasNode == effectNode {
		t.Fatal("opening a particle preset hid the editable canvas in the same tab group")
	}
	if effectNode != w.Tree.FindPanel(workspace.Inspector) {
		t.Fatal("particle preset is not docked with the inspector")
	}
	for _, item := range []struct {
		id   string
		node dock.NodeID
	}{{canvas, canvasNode}, {effect, effectNode}} {
		node, ok := w.Tree.Node(item.node)
		if !ok {
			t.Fatalf("missing tab group for %s", item.id)
		}
		if active, _ := node.ActivePanel(); active != item.id {
			t.Fatalf("%s is not visible: active tab is %s", item.id, active)
		}
	}
}

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
