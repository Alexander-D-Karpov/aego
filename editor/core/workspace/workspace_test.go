package workspace

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	"aego/editor/core/state"
)

type stub string

func (s stub) ID() string                       { return string(s) }
func (s stub) Title() string                    { return string(s) }
func (s stub) Draw(*services.Frame, mathx.Rect) {}

func register(t *testing.T, w *Workspace, list []Desc) {
	t.Helper()
	for _, d := range list {
		if err := w.Register(d); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResetLayoutRemovesSavedFile(t *testing.T) {
	root := t.TempDir()
	w := New(root, nil)
	register(t, w, []Desc{
		{Panel: stub(Hierarchy), Slot: SlotLeft},
		{Panel: stub(Assets), Slot: SlotBrowser},
		{Panel: stub(Scene), Slot: SlotCenter},
		{Panel: stub(Game), Slot: SlotCenter},
		{Panel: stub(Console), Slot: SlotBottom},
		{Panel: stub(Profiler), Slot: SlotBottom},
		{Panel: stub(Inspector), Slot: SlotRight},
	})
	w.ResetLayout()
	if err := state.SaveLayout(root, []byte("dock version=1 {\n}\n")); err != nil {
		t.Fatal(err)
	}
	w.Tree.Undock(Assets)
	w.ResetLayout()
	if _, ok := state.LoadLayout(root); ok {
		t.Fatal("reset must delete the saved layout")
	}
	if !w.Tree.Has(Assets) || !w.Tree.Has(Hierarchy) || !w.Tree.Has(Inspector) {
		t.Fatal("reset must restore every default panel")
	}
}

func TestLayoutAndShowFollowDescriptors(t *testing.T) {
	w := New(t.TempDir(), nil)
	register(t, w, []Desc{
		{Panel: stub(Inspector), Slot: SlotRight, Order: 50},
		{Panel: stub(Game), Slot: SlotCenter, Order: 30},
		{Panel: stub(Scene), Slot: SlotCenter, Order: 20},
		{Panel: stub("docs"), Near: Scene, Order: 40},
		{Panel: stub("quest"), Order: 10, Hidden: true},
	})
	w.ResetLayout()
	if !w.Tree.Has(Scene) || !w.Tree.Has(Game) || !w.Tree.Has(Inspector) {
		t.Fatal("slotted panels must be in the default layout")
	}
	if w.Tree.Has("docs") || w.Tree.Has("quest") {
		t.Fatal("panels without a slot must not be in the default layout")
	}
	if w.Tree.FindPanel(Scene) != w.Tree.FindPanel(Game) {
		t.Fatal("panels of one slot must share a tab group")
	}
	w.Show("docs")
	if w.Tree.FindPanel("docs") != w.Tree.FindPanel(Scene) {
		t.Fatal("Show must dock a panel next to its Near panel")
	}
	w.Tree.Undock(Game)
	w.Show(Game)
	if w.Tree.FindPanel(Game) != w.Tree.FindPanel(Scene) {
		t.Fatal("Show must return a slotted panel to its slot")
	}
	menu := w.Menu()
	want := []string{Scene, Game, "docs", Inspector}
	if len(menu) != len(want) {
		t.Fatalf("menu has %d panels, want %d", len(menu), len(want))
	}
	for i, id := range want {
		if menu[i].Panel.ID() != id {
			t.Fatalf("menu[%d] = %s, want %s", i, menu[i].Panel.ID(), id)
		}
	}
}

func TestRegisterRejectsAmbiguousPlacement(t *testing.T) {
	w := New(t.TempDir(), nil)
	if err := w.Register(Desc{Panel: stub("x"), Slot: SlotLeft, Near: Scene}); err == nil {
		t.Fatal("a slot and a near panel together must be rejected")
	}
	if err := w.Register(Desc{}); err == nil {
		t.Fatal("a descriptor without a panel must be rejected")
	}
}
