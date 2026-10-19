package workspace

import (
	"testing"

	"aego/editor/core/state"
)

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
