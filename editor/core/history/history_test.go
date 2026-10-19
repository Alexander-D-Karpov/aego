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

func TestSeekWalksSteps(t *testing.T) {
	c := &counter{}
	h := New(nil)
	for _, d := range []int{1, 2, 4} {
		h.Exec(&add{c: c, delta: d})
	}
	h.Undo()
	if got := h.Steps(nil); len(got) != 3 || h.Cursor() != 2 {
		t.Fatalf("steps=%v cursor=%d", got, h.Cursor())
	}
	if err := h.Seek(0); err != nil || c.value != 0 || h.Cursor() != 0 {
		t.Fatalf("seek 0: value=%d err=%v", c.value, err)
	}
	if err := h.Seek(3); err != nil || c.value != 7 {
		t.Fatalf("seek 3: value=%d err=%v", c.value, err)
	}
	if err := h.Seek(99); err != nil || h.Cursor() != 3 {
		t.Fatal("seek past the end must clamp")
	}
	h.SetLocked("locked")
	if err := h.Seek(1); err == nil || c.value != 7 {
		t.Fatal("seek must refuse a locked history")
	}
	h.SetLocked("")
	tx := h.Begin("open")
	if err := h.Seek(1); err == nil {
		t.Fatal("seek must refuse an open transaction")
	}
	tx.Abort()
}

func TestWalkSkipsUndone(t *testing.T) {
	c := &counter{}
	h := New(nil)
	a, b := &add{c: c, delta: 1}, &add{c: c, delta: 2}
	h.Exec(a)
	h.Exec(b)
	h.Undo()
	var seen []Command
	h.Walk(func(cmd Command) { seen = append(seen, cmd) })
	if len(seen) != 1 || seen[0] != a {
		t.Fatalf("walk saw %d commands", len(seen))
	}
}
