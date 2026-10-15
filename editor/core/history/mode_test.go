package history

import (
	"testing"

	"aego/scene"
)

type modeTarget struct{}

func (modeTarget) Scene() *scene.Scene { return nil }

type modeCmd struct {
	n *int
}

func (c *modeCmd) Label() string      { return "Bump" }
func (c *modeCmd) Do(Target) error    { *c.n++; return nil }
func (c *modeCmd) Undo(Target) error  { *c.n--; return nil }
func (c *modeCmd) Merge(Command) bool { return false }

func TestPlayModeNeverDirty(t *testing.T) {
	n := 0
	h := New(modeTarget{})
	h.SetMode(ModePlay)
	if err := h.Exec(&modeCmd{n: &n}); err != nil {
		t.Fatal(err)
	}
	if h.Dirty() {
		t.Fatal("play history must never be dirty")
	}
	h.MarkSaved()
	if u, _ := h.Labels(1); len(u) != 1 || u[0] != "[Play] Bump" {
		t.Fatalf("labels = %v", u)
	}
	if err := h.Undo(); err != nil || n != 0 {
		t.Fatalf("undo failed: n=%d err=%v", n, err)
	}
}

func TestDocumentModeStillDirty(t *testing.T) {
	n := 0
	h := New(modeTarget{})
	h.Exec(&modeCmd{n: &n})
	if !h.Dirty() {
		t.Fatal("document history must become dirty")
	}
	if u, _ := h.Labels(1); u[0] != "Bump" {
		t.Fatalf("labels = %v", u)
	}
}
