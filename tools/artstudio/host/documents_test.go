package host

import (
	"bytes"
	"context"
	"image"
	"testing"

	"aego/art"
)

func document(t *testing.T, h *Documents, name string) string {
	t.Helper()
	d, err := art.New(8, 8, art.RGBA, art.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := h.Open(d, name, false)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
func session(t *testing.T, h *Documents, key string) *art.Session {
	t.Helper()
	s, err := h.Session(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func draw(t *testing.T, s *art.Session, x int) {
	t.Helper()
	_, err := s.Execute(context.Background(), art.Operation{Kind: "stroke", Points: []image.Point{{X: x, Y: 1}}, Brush: art.Brush{Size: 1, Paint: art.Paint{Color: art.Color{R: 220, A: 255}}}})
	if err != nil {
		t.Fatal(err)
	}
}
func export(t *testing.T, h *Documents, key string) Export {
	t.Helper()
	out, err := h.Export(key, "aeart", 1)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTabsRetainIndependentHistoryAndPixels(t *testing.T) {
	h := New()
	a := document(t, h, "first.aeart")
	sa := session(t, h, a)
	draw(t, sa, 1)
	before, err := sa.Document.PNG(sa.Frame, art.Albedo)
	if err != nil {
		t.Fatal(err)
	}
	b := document(t, h, "second.aeart")
	sb := session(t, h, b)
	draw(t, sb, 4)
	if err = h.Select(a); err != nil {
		t.Fatal(err)
	}
	after, _ := sa.Document.PNG(sa.Frame, art.Albedo)
	if !bytes.Equal(before, after) || sa.Info().Undo != 1 || sb.Info().Undo != 1 {
		t.Fatal("switch lost pixel/history state")
	}
	if err = sa.Undo(); err != nil {
		t.Fatal(err)
	}
	if sb.Info().Undo != 1 || !sb.Info().Dirty {
		t.Fatal("undo crossed document boundary")
	}
}
func TestSaveReceiptTargetsOriginalDocumentAndRevision(t *testing.T) {
	h := New()
	a := document(t, h, "A.aeart")
	sa := session(t, h, a)
	draw(t, sa, 1)
	out := export(t, h, a)
	if err := h.BeginWrite(out.Receipt); err != nil {
		t.Fatal(err)
	}
	draw(t, sa, 2)
	b := document(t, h, "B.aeart")
	sb := session(t, h, b)
	draw(t, sb, 4)
	if err := h.Saved(out.Receipt); err != nil {
		t.Fatal(err)
	}
	if !sa.Info().Dirty || !sb.Info().Dirty {
		t.Fatal("late save cleared newer pixels or another tab")
	}
	if err := sa.Undo(); err != nil {
		t.Fatal(err)
	}
	if sa.Info().Dirty {
		t.Fatal("undo to actually saved snapshot must be clean")
	}
	if err := h.Saved(out.Receipt); err == nil {
		t.Fatal("receipt reused")
	}
}
func TestCloseRefusesDirtyAndRevokesPendingSaves(t *testing.T) {
	h := New()
	a := document(t, h, "A.aeart")
	s := session(t, h, a)
	draw(t, s, 1)
	out := export(t, h, a)
	if err := h.Close(a, false); err == nil {
		t.Fatal("discarded dirty document")
	}
	if len(h.Tabs()) != 1 {
		t.Fatal("refused close changed tabs")
	}
	if err := h.Close("", true); err != nil {
		t.Fatal(err)
	}
	if h.Active() != "" || len(h.Tabs()) != 0 {
		t.Fatal("closed default tab remains active")
	}
	if err := h.Saved(out.Receipt); err == nil {
		t.Fatal("late receipt published after close")
	}
}
func TestCancelledSaveDoesNotClearDirty(t *testing.T) {
	h := New()
	key := document(t, h, "A")
	s := session(t, h, key)
	draw(t, s, 1)
	out := export(t, h, key)
	h.CancelSave(out.Receipt)
	if err := h.Saved(out.Receipt); err == nil {
		t.Fatal("cancelled save accepted")
	}
	if !s.Info().Dirty {
		t.Fatal("cancelled save lost dirty marker")
	}
}
func TestReceiptAndDocumentLimits(t *testing.T) {
	h := New()
	key := document(t, h, "A")
	for n := 0; n < MaxPendingSaves; n++ {
		export(t, h, key)
	}
	if _, err := h.Export(key, "aeart", 1); err == nil {
		t.Fatal("unbounded retained snapshots")
	}
	if out, err := h.Export(key, "recovery", 1); err != nil || out.Receipt != "" {
		t.Fatal("recovery consumes save receipts", err)
	}
	for n := 1; n < MaxDocuments; n++ {
		document(t, h, "tab")
	}
	extra, _ := art.New(8, 8, art.RGBA, art.Limits{})
	defer extra.Close()
	if _, err := h.Open(extra, "extra", false); err == nil {
		t.Fatal("unbounded open docs")
	}
}
func TestCopyPasteAcrossOpenDocuments(t *testing.T) {
	h := New()
	a := document(t, h, "A")
	sa := session(t, h, a)
	draw(t, sa, 1)
	if err := h.Copy(a); err != nil {
		t.Fatal(err)
	}
	b := document(t, h, "B")
	sb := session(t, h, b)
	if err := h.Paste(b, image.Point{}); err != nil {
		t.Fatal(err)
	}
	ca, _ := sa.Document.Pixel(sa.ActiveCel(), 1, 1)
	cb, _ := sb.Document.Pixel(sb.ActiveCel(), 1, 1)
	if ca != cb || cb.A != 255 {
		t.Fatal("clipboard did not cross documents")
	}
	if err := sb.Undo(); err != nil {
		t.Fatal(err)
	}
	ca, _ = sa.Document.Pixel(sa.ActiveCel(), 1, 1)
	if ca.A != 255 {
		t.Fatal("paste undo damaged source")
	}
}
func TestCannotSwitchMidGestureOrShareMutableSession(t *testing.T) {
	h := New()
	a := document(t, h, "A")
	b := document(t, h, "B")
	s := session(t, h, b)
	if err := s.BeginGesture("pencil", image.Point{}, art.Brush{Size: 1}, false); err != nil {
		t.Fatal(err)
	}
	if err := h.Select(a); err == nil {
		t.Fatal("switched mid gesture")
	}
	s.CancelGesture()
	if err := h.Select(a); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Open(s.Document, "same pointer", false); err == nil {
		t.Fatal("one document acquired two histories")
	}
}

func TestDeferredSaveRefusesStalePickerAndClosedDocument(t *testing.T) {
	h := New()
	key := document(t, h, "A")
	s := session(t, h, key)
	draw(t, s, 1)
	out := export(t, h, key)
	draw(t, s, 2)
	if err := h.BeginWrite(out.Receipt); err != art.ErrConflict {
		t.Fatalf("stale picker: %v", err)
	}
	h.CancelSave(out.Receipt)
	out = export(t, h, key)
	if err := h.Close(key, true); err != nil {
		t.Fatal(err)
	}
	if err := h.BeginWrite(out.Receipt); err == nil {
		t.Fatal("write started after close")
	}
}
func TestWritePinsOnlyItsDocumentAndPreservesConcurrentEdits(t *testing.T) {
	h := New()
	a := document(t, h, "A")
	sa := session(t, h, a)
	draw(t, sa, 1)
	out := export(t, h, a)
	if err := h.BeginWrite(out.Receipt); err != nil {
		t.Fatal(err)
	}
	if err := h.BeginWrite(out.Receipt); err == nil {
		t.Fatal("write started twice")
	}
	if err := h.Close(a, true); err == nil {
		t.Fatal("closed pending writer")
	}
	b := document(t, h, "B")
	if err := h.Close(b, true); err != nil {
		t.Fatal("unrelated tab is pinned", err)
	}
	draw(t, sa, 2)
	if err := h.Saved(out.Receipt); err != nil {
		t.Fatal(err)
	}
	if !sa.Info().Dirty {
		t.Fatal("concurrent edit marked saved")
	}
	if err := h.Close(a, true); err != nil {
		t.Fatal(err)
	}
}

func TestPaletteReadTargetsOriginalTabAndRejectsStaleWork(t *testing.T) {
	h := New()
	a := document(t, h, "A")
	sa := session(t, h, a)
	revision := sa.Document.Revision()
	b := document(t, h, "B")
	sb := session(t, h, b)
	palette := []byte("GIMP Palette\n10 20 30 Name\n")
	if err := h.ImportPalette(a, revision, palette, "gpl", false); err != nil {
		t.Fatal(err)
	}
	if !sa.Info().Dirty || sb.Info().Dirty || h.Active() != b {
		t.Fatal("palette import changed the active tab instead of its owner")
	}
	if err := h.ImportPalette(a, revision, palette, "gpl", false); err != art.ErrConflict {
		t.Fatal("stale palette read was accepted", err)
	}
	out, err := h.Export(a, "palette", 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := art.DecodePalette(out.Bytes, "json")
	if err != nil || len(p) != 2 || p[1].Name != "Name" {
		t.Fatal(p, err)
	}
	if err = h.Close(a, true); err != nil {
		t.Fatal(err)
	}
	if err = h.ImportPalette(a, revision, palette, "gpl", false); err == nil {
		t.Fatal("closed palette read published")
	}
}

func TestSaveAcknowledgementRequiresStartedWrite(t *testing.T) {
	h := New()
	a := document(t, h, "A")
	s := session(t, h, a)
	draw(t, s, 1)
	out := export(t, h, a)
	if err := h.Saved(out.Receipt); err == nil {
		t.Fatal("export alone marked document saved")
	}
	if !s.Info().Dirty {
		t.Fatal("download lost dirty state")
	}
	if err := h.BeginWrite(out.Receipt); err != nil {
		t.Fatal(err)
	}
	if err := h.Saved(out.Receipt); err != nil {
		t.Fatal(err)
	}
	if s.Info().Dirty {
		t.Fatal("successful write was not recorded")
	}
}
