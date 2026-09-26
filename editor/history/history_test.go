package history

import (
	"errors"
	"testing"
)

type counter struct {
	value int
}

type add struct {
	c     *counter
	delta int
	fail  bool
}

func (a *add) Label() string { return "add" }

func (a *add) Do(Target) error {
	if a.fail {
		return errors.New("boom")
	}
	a.c.value += a.delta
	return nil
}

func (a *add) Undo(Target) error {
	a.c.value -= a.delta
	return nil
}

func (a *add) Merge(next Command) bool {
	n, ok := next.(*add)
	if !ok || n.c != a.c {
		return false
	}
	a.delta += n.delta
	return true
}

func TestExecUndoRedo(t *testing.T) {
	c := &counter{}
	h := New(nil)
	h.Exec(&add{c: c, delta: 2})
	h.Exec(&add{c: c, delta: 3})
	if c.value != 5 || h.Cursor() != 2 {
		t.Fatalf("value=%d cursor=%d", c.value, h.Cursor())
	}
	h.Undo()
	if c.value != 2 {
		t.Fatalf("after undo %d", c.value)
	}
	h.Redo()
	if c.value != 5 {
		t.Fatalf("after redo %d", c.value)
	}
}

func TestTransactionIsOneStepAndMerges(t *testing.T) {
	c := &counter{}
	h := New(nil)
	tx := h.Begin("drag")
	for i := 0; i < 10; i++ {
		tx.Exec(&add{c: c, delta: 1})
	}
	if tx.Len() != 1 {
		t.Fatalf("merge failed: %d commands", tx.Len())
	}
	tx.Commit()
	if h.Cursor() != 1 || c.value != 10 {
		t.Fatalf("cursor=%d value=%d", h.Cursor(), c.value)
	}
	h.Undo()
	if c.value != 0 {
		t.Fatal("one undo must revert the whole drag")
	}
}

func TestNestedBeginCommitsOnce(t *testing.T) {
	c := &counter{}
	h := New(nil)
	outer := h.Begin("outer")
	inner := h.Begin("inner")
	inner.Exec(&add{c: c, delta: 1})
	inner.Commit()
	if !h.InTransaction() {
		t.Fatal("inner commit closed the outer transaction")
	}
	outer.Commit()
	if h.Cursor() != 1 {
		t.Fatalf("cursor = %d", h.Cursor())
	}
}

func TestAbortRollsBack(t *testing.T) {
	c := &counter{}
	h := New(nil)
	tx := h.Begin("x")
	tx.Exec(&add{c: c, delta: 4})
	tx.Abort()
	if c.value != 0 || h.Cursor() != 0 || h.InTransaction() {
		t.Fatalf("abort left state: value=%d cursor=%d", c.value, h.Cursor())
	}
	if err := h.Exec(&add{c: c, delta: 1, fail: true}); err == nil {
		t.Fatal("failed command must return its error")
	}
	if h.Cursor() != 0 {
		t.Fatal("failed command must not enter history")
	}
}

func TestDirtyTracksSavedState(t *testing.T) {
	c := &counter{}
	h := New(nil)
	if h.Dirty() {
		t.Fatal("fresh history is dirty")
	}
	h.Exec(&add{c: c, delta: 1})
	if !h.Dirty() {
		t.Fatal("exec must dirty")
	}
	h.MarkSaved()
	h.Exec(&add{c: c, delta: 1})
	h.Undo()
	if h.Dirty() {
		t.Fatal("undo back to the saved state must be clean")
	}
	h.Undo()
	if !h.Dirty() {
		t.Fatal("undo past the saved state must be dirty")
	}
	h.Redo()
	if h.Dirty() {
		t.Fatal("redo to the saved state must be clean")
	}
	h.Undo()
	h.Exec(&add{c: c, delta: 7})
	if !h.Dirty() {
		t.Fatal("a branch that drops the saved state must stay dirty")
	}
}

func TestLimitKeepsSavedStateReachable(t *testing.T) {
	c := &counter{}
	h := New(nil)
	h.SetLimit(2)
	h.Exec(&add{c: c, delta: 1})
	h.MarkSaved()
	h.Exec(&add{c: c, delta: 1})
	h.Exec(&add{c: c, delta: 1})
	h.Undo()
	h.Undo()
	if h.Dirty() {
		t.Fatal("trimmed history lost the saved state")
	}
}
