package code

import (
	"bytes"
	"testing"
)

func TestVolatileSharesTextUndoAndRedo(t *testing.T) {
	d := New([]byte("gain=1\n"))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() || d.VolatileDirty() {
		t.Fatal("an empty attached state must be clean")
	}
	d.ReplaceSnapshot([]byte("gain=2\n"), false)
	input := []byte("private-audio-edit")
	if changed, err := d.ReplaceVolatile(d.Rev(), input); err != nil || !changed {
		t.Fatalf("volatile edit: changed=%v err=%v", changed, err)
	}
	input[0] = 'X'
	value, attached := d.Volatile()
	if !attached || string(value) != "private-audio-edit" || !d.VolatileDirty() {
		t.Fatal("state was not stored independently")
	}
	value[0] = 'Y'
	value, _ = d.Volatile()
	if string(value) != "private-audio-edit" || bytes.Contains(d.Bytes(), value) {
		t.Fatal("state leaked through a caller-owned slice or the text document")
	}
	d.ReplaceSnapshot([]byte("gain=3\n"), true)
	if !d.Undo() || string(d.Bytes()) != "gain=2\n" || !d.VolatileDirty() {
		t.Fatal("first Undo must restore only the latest text change")
	}
	if !d.Undo() || string(d.Bytes()) != "gain=2\n" || d.VolatileDirty() {
		t.Fatal("second Undo must restore only the volatile state")
	}
	if !d.Undo() || string(d.Bytes()) != "gain=1\n" || d.Dirty() || d.CanUndo() {
		t.Fatal("third Undo must restore the clean original")
	}
	if !d.Redo() || string(d.Bytes()) != "gain=2\n" || d.VolatileDirty() {
		t.Fatal("first Redo must restore the earlier text edit")
	}
	if !d.Redo() || string(d.Bytes()) != "gain=2\n" || !d.VolatileDirty() {
		t.Fatal("second Redo must restore the volatile edit")
	}
	if !d.Redo() || string(d.Bytes()) != "gain=3\n" || d.CanRedo() {
		t.Fatal("third Redo must restore the later text edit")
	}
	d.MarkSaved()
	if !d.Dirty() || !d.VolatileDirty() {
		t.Fatal("marking text saved must not mark volatile changes saved")
	}
}

func TestVolatileRejectsStaleOversizedAndExhaustedEdits(t *testing.T) {
	d := New([]byte("source"))
	if _, err := d.ReplaceVolatile(0, []byte("x")); err == nil {
		t.Fatal("unattached state accepted an edit")
	}
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if err := d.AttachVolatile(); err == nil {
		t.Fatal("a second owner replaced the attached state")
	}
	d.ReplaceSnapshot([]byte("new source"), false)
	if changed, err := d.ReplaceVolatile(0, []byte("x")); err == nil || changed {
		t.Fatal("stale text revision accepted a volatile edit")
	}
	if changed, err := d.ReplaceVolatile(d.Rev(), make([]byte, maxVolatileBytes+1)); err == nil || changed {
		t.Fatal("oversized state was accepted")
	}
	if state, _ := d.Volatile(); state != nil || !d.CanUndo() || d.CanRedo() {
		t.Fatal("refused edits changed state or text history")
	}
	d.rev = ^uint64(0)
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("x")); err == nil || changed {
		t.Fatal("revision exhaustion accepted a volatile edit")
	}
	if changed, err := d.ReplaceVolatile(d.Rev(), nil); err != nil || changed {
		t.Fatal("an unchanged state should remain a no-op at exhaustion")
	}
}

func TestVolatileBudgetAndRedoRelease(t *testing.T) {
	d := New(nil)
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if d.volatile.budget != maxVolatileHistoryBytes {
		t.Fatal("production volatile history budget changed")
	}
	d.volatile.budget = 8
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("1234")); err != nil || !changed {
		t.Fatalf("boundary edit: %v %v", changed, err)
	}
	rev := d.Rev()
	if changed, err := d.ReplaceVolatile(rev, []byte("x")); err == nil || changed || d.Rev() != rev {
		t.Fatal("history-budget failure changed the journal")
	}
	if !d.Undo() || d.VolatileDirty() || !d.CanRedo() {
		t.Fatal("cannot undo the boundary edit")
	}
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("abcd")); err != nil || !changed || d.CanRedo() {
		t.Fatalf("new edit did not replace redo history: %v %v", changed, err)
	}
	for _, g := range d.redo[:cap(d.redo)] {
		if g.volatileBefore != nil || g.volatileAfter != nil {
			t.Fatal("discarded redo still retains a volatile snapshot")
		}
	}
}

func TestVolatileRevisionsPreserveTextSpanMapping(t *testing.T) {
	d := New([]byte("abcdef"))
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	original := d.Rev()
	if changed, err := d.ReplaceVolatile(original, []byte("edit")); err != nil || !changed {
		t.Fatal("cannot set volatile state", err)
	}
	if !d.Undo() || !d.Redo() {
		t.Fatal("cannot replay volatile edit")
	}
	if a, b, ok := d.MapSpan(original, 2, 5); !ok || a != 2 || b != 5 {
		t.Fatalf("volatile revisions changed text mapping: %d %d %v", a, b, ok)
	}
	if off, ok := d.Unmap(original, 3); !ok || off != 3 {
		t.Fatalf("volatile revisions changed reverse mapping: %d %v", off, ok)
	}
	for i := 0; i < journalLimit+2; i++ {
		if changed, err := d.ReplaceVolatile(d.Rev(), []byte{byte(1 + i%2)}); err != nil || !changed {
			t.Fatal("cannot advance volatile revision", err)
		}
	}
	if _, _, ok := d.MapSpan(original, 2, 5); ok {
		t.Fatal("mapping accepted a revision outside the bounded journal")
	}
	if a, b, ok := d.MapSpan(d.Rev()-journalLimit, 2, 5); !ok || a != 2 || b != 5 {
		t.Fatalf("mapping rejected the oldest retained revision: %d %d %v", a, b, ok)
	}
}

func TestVolatileUndoRedoRefuseRevisionOverflow(t *testing.T) {
	d := New(nil)
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReplaceVolatile(0, []byte("edit")); err != nil {
		t.Fatal(err)
	}
	d.rev = ^uint64(0)
	if d.Undo() || !d.CanUndo() || d.CanRedo() || !d.VolatileDirty() {
		t.Fatal("overflowing Undo changed volatile history")
	}
	d.rev--
	if !d.Undo() || !d.CanRedo() || d.VolatileDirty() {
		t.Fatal("cannot prepare redo at the revision limit")
	}
	if d.Redo() || !d.CanRedo() || d.CanUndo() || d.VolatileDirty() {
		t.Fatal("overflowing Redo changed volatile history")
	}
}
