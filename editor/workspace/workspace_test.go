package workspace

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/state"
)

type stub string

func (s stub) ID() string                       { return string(s) }
func (s stub) Title() string                    { return string(s) }
func (s stub) Draw(*context.Editor, mathx.Rect) {}

func TestResetLayoutRemovesSavedFile(t *testing.T) {
	root := t.TempDir()
	w := New(root, nil)
	for _, id := range []string{Hierarchy, Scene, Game, Inspector, Assets, Console, Profiler} {
		if err := w.Register(stub(id)); err != nil {
			t.Fatal(err)
		}
	}
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
