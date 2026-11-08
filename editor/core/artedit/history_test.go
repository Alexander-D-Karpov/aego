package artedit

import (
	"context"
	"errors"
	"image"
	"testing"

	"aego/art"
	"aego/editor/core/document"
)

// These integration tests use the real editor History, not a stub. They require
// the full Aego tree and its toolchain; the standalone harness cannot run them.
func boundForTest(t *testing.T) *Document {
	t.Helper()
	d, err := art.New(16, 16, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	b := &Document{Asset: &document.AssetDocument{Capabilities: document.EditSource}, limits: art.DefaultLimits()}
	if err = b.install(d, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}
func TestArtUsesEditorHistoryForUserAndAgent(t *testing.T) {
	b := boundForTest(t)
	info := b.Session.Document.Info()
	frame, layer := info.Frames[0].ID, info.Layers[0].ID
	ops := []art.Operation{{Kind: "stroke", Frame: frame, Layer: layer, Points: []image.Point{{1, 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 255, A: 255}}}}, {Kind: "add-frame", Frame: frame, As: "next", DurationMS: 80}, {Kind: "copy-cel", Linked: true, Layer: layer, Frame: "$next", Cel: b.Session.Document.Info().Cels[0].ID}}
	// The batch uses linked cels from the actual document model.
	proposal, err := b.Propose(context.Background(), info.Revision, "agent frames", ops)
	if err != nil {
		t.Fatal(err)
	}
	if b.History.CanUndo() {
		t.Fatal("proposal applied before approval")
	}
	if err = b.Accept(proposal); err != nil {
		t.Fatal(err)
	}
	if b.History.Cursor() != 1 || b.Session.Info().Undo != 1 {
		t.Fatal("agent did not use common history")
	}
	if _, err = b.Execute(context.Background(), art.Operation{Kind: "duration", Frame: frame, DurationMS: 120}); err != nil {
		t.Fatal(err)
	}
	if b.History.Cursor() != 2 {
		t.Fatal("user did not use same history")
	}
	if err = b.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if err = b.SessionUndo(); err != nil {
		t.Fatal(err)
	}
	if len(b.Session.Document.Info().Frames) != 1 || b.Session.Info().Dirty {
		t.Fatal("undo did not restore saved drawing")
	}
	if err = b.SessionRedo(); err != nil {
		t.Fatal(err)
	}
	if len(b.Session.Document.Info().Frames) != 2 {
		t.Fatal("redo lost links/frames")
	}
}
func TestArtProposalsRejectClosedAndRevisedDocuments(t *testing.T) {
	b := boundForTest(t)
	p, err := b.Propose(context.Background(), b.Session.Document.Revision(), "agent", []art.Operation{{Kind: "add-frame", DurationMS: 90}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Execute(context.Background(), art.Operation{Kind: "duration", DurationMS: 120}); err != nil {
		t.Fatal(err)
	}
	if err = b.Accept(p); !errors.Is(err, art.ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	b.Close()
	if err = b.Accept(p); !errors.Is(err, art.ErrClosed) {
		t.Fatalf("closed: %v", err)
	}
}
