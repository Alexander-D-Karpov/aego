package artedit

import (
	"context"
	"errors"
	"testing"

	"aego/art"
)

// These tests require real editor/core/history and document packages. They must
// run in the full native tree, not a replacement/stub editor test harness.
func TestPreviewGuardIncludesReplacedDocumentAndClose(t *testing.T) {
	b := boundForTest(t)
	b.Asset.Binding.Model = b
	before, generation, err := b.PreviewSnapshot()
	if err != nil || len(before) == 0 || !b.PreviewCurrent(generation) {
		t.Fatalf("capture: %v", err)
	}
	replacement, err := art.Decode(before, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err = b.install(replacement, true); err != nil {
		t.Fatal(err)
	}
	if b.PreviewCurrent(generation) {
		t.Fatal("pre-recovery candidate may publish into a replacement document")
	}
	_, current, err := b.PreviewSnapshot()
	if err != nil || current <= generation {
		t.Fatalf("replacement generation: %d after %d: %v", current, generation, err)
	}
	b.Close()
	if b.PreviewCurrent(current) {
		t.Fatal("closed binding accepted a candidate")
	}
	if _, _, err := b.PreviewSnapshot(); !errors.Is(err, art.ErrClosed) {
		t.Fatalf("closed snapshot: %v", err)
	}
}

func TestPreviewGuardRejectsUndoAndOtherBinding(t *testing.T) {
	b := boundForTest(t)
	b.Asset.Binding.Model = b
	_, first, err := b.PreviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Execute(context.Background(), art.Operation{Kind: "duration", DurationMS: 120}); err != nil {
		t.Fatal(err)
	}
	if b.PreviewCurrent(first) {
		t.Fatal("changed drawing accepted an old candidate")
	}
	_, edited, err := b.PreviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = b.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if b.PreviewCurrent(edited) || b.PreviewCurrent(first) {
		t.Fatal("Undo must invalidate captures even when pixels match an old state")
	}
	_, undone, err := b.PreviewSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	b.Asset.Binding.Model = nil
	if b.PreviewCurrent(undone) {
		t.Fatal("detached binding accepted publication")
	}
}
