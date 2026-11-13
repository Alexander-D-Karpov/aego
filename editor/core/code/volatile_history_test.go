package code

import "testing"

func TestVolatileHistoryTracksCurrentUndoAndRedo(t *testing.T) {
	var absent *Doc
	if absent.HasVolatileHistory() {
		t.Fatal("nil document reported structured history")
	}
	d := New([]byte("saved\n"))
	d.ReplaceSnapshot([]byte("text edit\n"), false)
	if d.HasVolatileHistory() {
		t.Fatal("unattached text history was classified as structured history")
	}
	if err := d.AttachVolatile(); err != nil {
		t.Fatal(err)
	}
	if d.HasVolatileHistory() {
		t.Fatal("text history alone was classified as structured history")
	}
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("private")); err != nil || !changed || !d.HasVolatileHistory() {
		t.Fatal("current structured state was not reported", err)
	}
	if !d.Undo() || d.VolatileDirty() || !d.HasVolatileHistory() || !d.CanRedo() {
		t.Fatal("structured redo was not reported")
	}
	d.ReplaceSnapshot([]byte("another text edit\n"), false)
	if d.CanRedo() || d.HasVolatileHistory() {
		t.Fatal("discarded redo left phantom structured history")
	}
	if changed, err := d.ReplaceVolatile(d.Rev(), []byte("private")); err != nil || !changed {
		t.Fatal("cannot create structured state", err)
	}
	if changed, err := d.ReplaceVolatile(d.Rev(), nil); err != nil || !changed {
		t.Fatal("cannot clear current structured state", err)
	}
	if d.VolatileDirty() || !d.HasVolatileHistory() || !d.CanUndo() {
		t.Fatal("structured undo was not reported")
	}
}
