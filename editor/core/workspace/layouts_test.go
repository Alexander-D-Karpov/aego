package workspace

import (
	"testing"

	"aego/editor/core/state"
	"aego/ui/dock"
)

type scopedStub struct {
	stub
	scope string
}

func (s scopedStub) Scope() string { return s.scope }

func standard() []Desc {
	return []Desc{
		{Panel: stub(Hierarchy), Slot: SlotLeft, Order: 10},
		{Panel: stub(Assets), Slot: SlotBrowser, Order: 20},
		{Panel: stub(Scene), Slot: SlotCenter, Order: 30},
		{Panel: stub(Game), Slot: SlotCenter, Order: 40},
		{Panel: stub(Console), Slot: SlotBottom, Order: 50},
		{Panel: stub(Inspector), Slot: SlotRight, Order: 60},
	}
}

func withPresets(t *testing.T, root string) *Workspace {
	t.Helper()
	w := New(root, nil)
	register(t, w, standard())
	for _, p := range []Preset{
		{ID: Design, Title: "Design"},
		{ID: Programming, Title: "Programming", Order: 10, Slots: map[string]Slot{Hierarchy: SlotNone, Game: SlotNone}},
	} {
		if err := w.RegisterPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func TestWorkspacesRoundTrip(t *testing.T) {
	root := t.TempDir()
	w := withPresets(t, root)
	w.LoadLayout()
	if w.Current().ID != Design {
		t.Fatalf("first workspace = %s", w.Current().ID)
	}
	w.Tree.Undock(Console)
	if !w.Switch(Programming) {
		t.Fatal("switch failed")
	}
	if w.Tree.Has(Hierarchy) || w.Tree.Has(Game) || !w.Tree.Has(Console) {
		t.Fatal("programming must follow its own slots")
	}
	id, err := w.SaveAs("Mine")
	if err != nil {
		t.Fatal(err)
	}
	w.SetToolbar(false)
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	r := withPresets(t, root)
	r.LoadLayout()
	if r.preset(id) == nil || r.Current().ID != Design {
		t.Fatal("user workspace must load and the first workspace stays current")
	}
	if r.Tree.Has(Console) {
		t.Fatal("design must keep its own tree")
	}
	if !r.Switch(id) || r.Toolbar() || r.Tree.Has(Hierarchy) {
		t.Fatal("user workspace must keep its tree and toolbar")
	}
}

// The new factory split must not migrate a user's previously saved tab group.
// Only a deliberate Reset selects the new factory geometry.
func TestArtFactorySplitPreservesSavedLayout(t *testing.T) {
	root := t.TempDir()
	makeWorkspace := func() *Workspace {
		w := New(root, nil)
		register(t, w, []Desc{
			{Panel: stub(Scene), Slot: SlotCenter},
			{Panel: stub(Assets), Slot: SlotBottom},
			{Panel: stub("art.timeline"), Slot: SlotBottom},
		})
		if err := w.RegisterPreset(Preset{
			ID: Art, Explicit: true,
			Slots: map[string]Slot{Scene: SlotCenter, Assets: SlotBottom, "art.timeline": SlotBottom},
			Splits: []PanelSplit{
				{Panel: "art.timeline", Anchor: Scene, Side: dock.SideBottom, Ratio: .72},
				{Panel: Assets, Anchor: "art.timeline", Side: dock.SideLeft, Ratio: .32},
			},
		}); err != nil {
			t.Fatal(err)
		}
		w.LoadLayout()
		return w
	}
	w := makeWorkspace()
	if w.Tree.FindPanel(Assets) == w.Tree.FindPanel("art.timeline") {
		t.Fatal("new Art layout did not split Assets and Timeline")
	}
	w.Tree.DockPanel(Assets, w.Tree.FindPanel("art.timeline"), dock.SideCenter)
	w.Tree.Activate(Assets)
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	r := makeWorkspace()
	if r.Tree.FindPanel(Assets) != r.Tree.FindPanel("art.timeline") {
		t.Fatal("loading replaced the user's saved tab group")
	}
	group, ok := r.Tree.Node(r.Tree.FindPanel(Assets))
	if !ok {
		t.Fatal("saved Assets group is missing")
	}
	if active, _ := group.ActivePanel(); active != Assets {
		t.Fatalf("loading changed the saved active tab: %q", active)
	}
	r.ResetLayout()
	assets, timeline := r.Tree.FindPanel(Assets), r.Tree.FindPanel("art.timeline")
	if assets == 0 || timeline == 0 || assets == timeline {
		t.Fatal("explicit Reset did not restore separate factory docks")
	}
	a, _ := r.Tree.Node(assets)
	b, _ := r.Tree.Node(timeline)
	parent, ok := r.Tree.Node(a.Parent)
	if !ok || a.Parent != b.Parent || parent.Axis != dock.AxisX || parent.First != assets || parent.Second != timeline || parent.Ratio != .32 {
		t.Fatalf("unexpected Assets/Timeline split: %+v", parent)
	}
}

// A named workspace keeps its saved tree. An explicit Reset uses the same
// dynamic-document preference and factory proportions as its base, including
// after reloading the named workspace from disk.
func TestNamedWorkspaceResetInheritsAuthoringFactory(t *testing.T) {
	root := t.TempDir()
	makeWorkspace := func() *Workspace {
		w := New(root, nil)
		register(t, w, []Desc{
			{Panel: stub("tools"), Slot: SlotLeft},
			{Panel: stub(Scene), Slot: SlotCenter},
			{Panel: scopedStub{stub("drawing-a"), "drawing"}, Slot: SlotCenter},
			{Panel: scopedStub{stub("drawing-b"), "drawing"}, Slot: SlotCenter},
			{Panel: stub("layers"), Slot: SlotRight},
		})
		if err := w.RegisterPreset(Preset{
			ID: Art, Explicit: true,
			Slots:  map[string]Slot{"tools": SlotLeft, Scene: SlotCenter, "layers": SlotRight},
			Scopes: map[string]Slot{"drawing": SlotCenter}, Front: []string{Scene},
			FrontScopes: []string{"drawing"}, LeftRatio: .1, CenterRatio: .7,
		}); err != nil {
			t.Fatal(err)
		}
		w.LoadLayout()
		return w
	}
	w := makeWorkspace()
	w.Focus("drawing-a")
	id, err := w.SaveAs("My Art")
	if err != nil {
		t.Fatal(err)
	}
	checkPreset := func(w *Workspace) {
		t.Helper()
		p := w.Current()
		if p.LeftRatio != .1 || p.CenterRatio != .7 || len(p.FrontScopes) != 1 || p.FrontScopes[0] != "drawing" {
			t.Fatalf("named workspace lost its base factory settings: %+v", p)
		}
	}
	checkPreset(w)
	w.Tree.Undock("tools")
	w.Tree.Activate(Scene)
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	r := makeWorkspace()
	if !r.Switch(id) {
		t.Fatal("named workspace did not reload")
	}
	checkPreset(r)
	if r.Tree.Has("tools") {
		t.Fatal("reloading replaced the saved user layout")
	}
	n, ok := r.Tree.Node(r.Tree.FindPanel(Scene))
	if !ok {
		t.Fatal("saved scene tab is missing")
	}
	if active, _ := n.ActivePanel(); active != Scene {
		t.Fatalf("reloading replaced the saved active tab: %s", active)
	}
	r.Focus("drawing-a")
	r.ResetLayout()
	if !r.Tree.Has("tools") {
		t.Fatal("explicit reset did not restore the base layout")
	}
	n, ok = r.Tree.Node(r.Tree.FindPanel("drawing-a"))
	if !ok {
		t.Fatal("focused drawing is missing after reset")
	}
	if active, _ := n.ActivePanel(); active != "drawing-a" {
		t.Fatalf("reset did not preserve the focused drawing: %s", active)
	}
	rootNode, ok := r.Tree.Node(r.Tree.Root())
	if !ok || rootNode.Ratio != .1 {
		t.Fatalf("reset lost the base factory proportions: %+v", rootNode)
	}
}

func TestLegacyLayoutImported(t *testing.T) {
	root := t.TempDir()
	legacy := New(root, nil)
	register(t, legacy, standard())
	legacy.ResetLayout()
	legacy.Tree.Undock(Assets)
	if err := state.SaveLayout(root, legacy.Tree.Save()); err != nil {
		t.Fatal(err)
	}
	w := withPresets(t, root)
	w.LoadLayout()
	if w.Tree.Has(Assets) {
		t.Fatal("legacy layout must become the design tree")
	}
	if _, ok := state.LoadLayout(root); ok {
		t.Fatal("legacy layout file must be removed")
	}
	if _, ok := state.LoadWorkspaces(root); !ok {
		t.Fatal("workspaces file must be written")
	}
}

func TestResetRenameDelete(t *testing.T) {
	w := withPresets(t, t.TempDir())
	w.LoadLayout()
	w.Tree.Undock(Console)
	w.ResetLayout()
	if !w.Tree.Has(Console) {
		t.Fatal("reset must restore the preset layout")
	}
	if err := w.Delete(Design); err == nil {
		t.Fatal("built-in workspaces cannot be deleted")
	}
	id, err := w.SaveAs("Temp")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Rename(id, "Renamed"); err != nil || w.Current().Title != "Renamed" {
		t.Fatal("rename failed")
	}
	if err := w.Delete(id); err != nil {
		t.Fatal(err)
	}
	if w.Current().ID != Design || w.preset(id) != nil {
		t.Fatalf("delete must return to the base, current = %s", w.Current().ID)
	}
}

func TestUnknownWorkspaceKept(t *testing.T) {
	root := t.TempDir()
	data := []byte("workspaces version=1 {\n  workspace \"mod.sound\" title=\"Sound\" {\n  }\n}\n")
	if err := state.SaveWorkspaces(root, data); err != nil {
		t.Fatal(err)
	}
	w := withPresets(t, root)
	w.LoadLayout()
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	r := New(root, nil)
	register(t, r, standard())
	if err := r.RegisterPreset(Preset{ID: "mod.sound", Title: "x"}); err != nil {
		t.Fatal(err)
	}
	r.LoadLayout()
	if r.Current().Title != "Sound" {
		t.Fatal("a workspace of a module that was not loaded must survive a save")
	}
}

func TestMaximizeYieldsToShowAndSwitch(t *testing.T) {
	w := withPresets(t, t.TempDir())
	w.LoadLayout()
	w.Show(Game)
	before := string(w.Tree.Save())
	if !w.ToggleMaximize(Scene) {
		t.Fatal("scene must maximize")
	}
	w.Show(Scene)
	if w.Tree.Maximized() != Scene {
		t.Fatal("showing the maximized panel must keep it maximized")
	}
	w.Show(Game)
	if w.Tree.Maximized() != "" || string(w.Tree.Save()) != before {
		t.Fatal("showing another panel must restore the same tree")
	}
	w.ToggleMaximize(Scene)
	w.Switch(Programming)
	if w.Tree.Maximized() != "" {
		t.Fatal("a workspace switch must not carry the maximized panel")
	}
}

// A layout saved for an older revision of a built-in preset is dropped, so the new default shows.
func TestPresetRevisionDropsOldLayout(t *testing.T) {
	root := t.TempDir()
	w := withPresets(t, root)
	w.LoadLayout()
	w.Switch(Programming)
	w.Tree.Undock(Console)
	if err := w.SaveLayout(); err != nil {
		t.Fatal(err)
	}
	same := withPresets(t, root)
	same.LoadLayout()
	same.Switch(Programming)
	if same.Tree.Has(Console) {
		t.Fatal("a layout of the same revision must be kept")
	}
	newer := New(root, nil)
	register(t, newer, standard())
	for _, p := range []Preset{{ID: Design, Title: "Design"}, {ID: Programming, Title: "Programming", Order: 10, Rev: 1}} {
		if err := newer.RegisterPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	newer.LoadLayout()
	newer.Switch(Programming)
	if !newer.Tree.Has(Console) {
		t.Fatal("a layout saved before the preset's revision must be dropped")
	}
}
