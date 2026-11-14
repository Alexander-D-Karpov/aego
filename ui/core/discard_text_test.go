package core

import (
	"testing"

	"aego/core/mathx"
	"aego/platform/input"
)

func TestDiscardTextEditClearsActiveAndQueuedFocusLoss(t *testing.T) {
	c := New(nil)
	in := &input.State{}
	first, second, third := ID(1), ID(2), ID(3)
	c.SetFocus(first)
	c.EditBuffer(first, "first draft")
	c.SetFocus(second)
	c.EditBuffer(second, "second draft")
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	if text, ok := c.LostText(first); !ok || text != "first draft" {
		t.Fatal("test did not create a pending focus-loss commit")
	}
	c.SetFocus(third)
	c.EditBuffer(third, "third draft")
	c.WantTextInput(mathx.Rect{W: 20, H: 20}, 0)
	c.DiscardTextEdit()
	if c.Focus() != 0 || c.EditingID() != 0 {
		t.Fatal("active text edit survived document replacement")
	}
	if _, ok := c.LostText(first); ok {
		t.Fatal("current focus-loss text survived document replacement")
	}
	if want, _, _ := c.WantsTextInput(); want {
		t.Fatal("discarded edit still requests text input")
	}
	c.End()
	c.Begin(in, mathx.Vec2{X: 800, Y: 600}, 1)
	if text, ok := c.LostText(second); ok {
		t.Fatalf("queued focus-loss text survived document replacement: %q", text)
	}
	c.SetFocus(first)
	if got := c.EditBuffer(first, "fresh").String(); got != "fresh" {
		t.Fatalf("new edit reused old buffer: %q", got)
	}
}
