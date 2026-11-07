package context

import (
	"errors"
	"testing"

	"aego/editor/core/document"
)

func TestStructuredHistoryRoutesToSourceNotSidecar(t *testing.T) {
	calls := 0
	failure := errors.New("history test")
	d := &document.AssetDocument{Binding: document.AssetBinding{
		CanUndo: func() bool { return true },
		CanRedo: func() bool { return true },
		Undo:    func() error { calls++; return failure },
		Redo:    func() error { calls += 10; return nil },
	}}
	if handled, err := structuredAssetHistory(d, false); !handled || !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("undo: handled=%v err=%v calls=%d", handled, err, calls)
	}
	if handled, err := structuredAssetHistory(d, true); !handled || err != nil || calls != 11 {
		t.Fatalf("redo: handled=%v err=%v calls=%d", handled, err, calls)
	}
}

func TestImageViewerKeepsExistingHistoryRouting(t *testing.T) {
	d := &document.AssetDocument{Binding: document.AssetBinding{Undo: func() error {
		t.Fatal("legacy handler must use its existing routing")
		return nil
	}}}
	if handled, err := structuredAssetHistory(d, false); handled || err != nil {
		t.Fatalf("legacy handler was intercepted: %v %v", handled, err)
	}
}
