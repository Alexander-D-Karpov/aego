package code

import "testing"

func TestSnapshotGestureIsOneUndoAndDoesNotMergeWithTyping(t *testing.T) {
	d := New([]byte("original\n"))
	d.ReplaceSnapshot([]byte("value=0.1\n"), false)
	d.ReplaceSnapshot([]byte("value=0.2\n"), true)
	d.ReplaceSnapshot([]byte("value=0.3\n"), true)
	if !d.Dirty() || !d.Undo() || string(d.Bytes()) != "original\n" || d.CanUndo() {
		t.Fatal("slider did not become exactly one undo command")
	}
	if !d.Redo() || string(d.Bytes()) != "value=0.3\n" {
		t.Fatal("redo did not restore final slider value")
	}
	d.Replace(d.Len(), 0, "// typed\n")
	d.ReplaceSnapshot([]byte("value=0.4\n// typed\n"), false)
	if !d.Undo() || string(d.Bytes()) != "value=0.3\n// typed\n" {
		t.Fatal("material edit swallowed code editing")
	}
	if !d.Undo() || string(d.Bytes()) != "value=0.3\n" {
		t.Fatal("code edit lost its own undo entry")
	}
}
