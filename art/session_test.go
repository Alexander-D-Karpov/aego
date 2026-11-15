package art

import (
	"context"
	"errors"
	"image"
	"testing"
)

func sessionForTest(t *testing.T) (*Session, *Journal) {
	t.Helper()
	d, err := New(32, 32, RGBA, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	j := NewJournal(Limits{})
	s, err := NewSession(d, j)
	if err != nil {
		t.Fatal(err)
	}
	return s, j
}
func TestSessionGestureHistoryAndRecovery(t *testing.T) {
	s, _ := sessionForTest(t)
	b := Brush{Size: 1, Paint: Paint{Color: Color{255, 0, 0, 255}}}
	if err := s.BeginGesture("pencil", image.Pt(1, 1), b, false); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveGesture([]image.Point{{10, 1}}); err != nil {
		t.Fatal(err)
	}
	if !s.Info().Gesture || s.Info().Dirty {
		t.Fatal("draft was published")
	}
	draft, err := s.Preview("", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if draft.NRGBAAt(6, 1).A != 255 {
		t.Fatal("draft not visible")
	}
	if err := s.EndGesture(); err != nil {
		t.Fatal(err)
	}
	if s.Info().Undo != 1 || !s.Info().Dirty {
		t.Fatal("gesture not one history entry")
	}
	if err := s.MarkSaved(s.Document.Revision()); err != nil {
		t.Fatal(err)
	}
	if s.Info().Dirty {
		t.Fatal("saved document dirty")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if !s.Info().Dirty {
		t.Fatal("undo to pre-save incorrectly clean")
	}
	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if s.Info().Dirty {
		t.Fatal("redo to saved content dirty")
	}
	s.MarkUnsaved()
	if !s.Info().Dirty {
		t.Fatal("recovery must need explicit save")
	}
}
func TestShapeGestureRejectsConcurrentRevision(t *testing.T) {
	s, j := sessionForTest(t)
	b := Brush{Size: 1, Paint: Paint{Color: Color{2, 3, 4, 255}}}
	if err := s.BeginGesture("rectangle", image.Pt(1, 1), b, true); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveGesture([]image.Point{{4, 4}}); err != nil {
		t.Fatal(err)
	}
	c, _, err := s.Document.Propose(context.Background(), s.Document.Revision(), "agent", []Operation{{Kind: "duration", Frame: s.Frame, DurationMS: 150}})
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Exec(c); err != nil {
		t.Fatal(err)
	}
	if err = s.MoveGesture([]image.Point{{6, 6}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	if s.Info().Gesture {
		t.Fatal("stale gesture retained")
	}
	im, err := s.Preview("", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(2, 2).A != 0 {
		t.Fatal("stale shape published")
	}
	if s.Info().Undo != 1 {
		t.Fatal("gesture contaminated agent history")
	}
}
func TestSessionLinkedFramesAndUndoSelectionRepair(t *testing.T) {
	s, _ := sessionForTest(t)
	frame := s.Frame
	id, err := s.Execute(context.Background(), Operation{Kind: "duplicate-frame", Linked: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Frame != id || id == frame {
		t.Fatal("new frame not selected")
	}
	if err = s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.Frame != frame {
		t.Fatal("undo did not repair selected frame")
	}
	if err = s.Select("", id, ""); err == nil {
		t.Fatal("selected deleted frame")
	}
	if err = s.Redo(); err != nil {
		t.Fatal(err)
	}
	if err = s.Select("", id, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Execute(context.Background(), Operation{Kind: "unlink"}); err != nil {
		t.Fatal(err)
	}
	i := s.Document.Info()
	if i.Cels[0].Image == i.Cels[1].Image {
		t.Fatal("unlink failed")
	}
}
func TestSessionSelectedPasteAndMode(t *testing.T) {
	s, _ := sessionForTest(t)
	_, err := s.Execute(context.Background(), Operation{Kind: "rectangle", Points: []image.Point{{1, 1}, {3, 3}}, Filled: true, Brush: Brush{Size: 1, Paint: Paint{Color: Color{255, 0, 0, 255}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetSelection("rectangle", []image.Point{{1, 1}, {3, 3}}, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Copy(); err != nil {
		t.Fatal(err)
	}
	if err = s.SetSelection("none", nil, 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Paste(image.Pt(10, 10)); err != nil {
		t.Fatal(err)
	}
	c, err := s.Document.Pixel(s.ActiveCel(), 11, 11)
	if err != nil || c.A != 255 {
		t.Fatalf("paste %v %v", c, err)
	}
	if _, err = s.Execute(context.Background(), Operation{Kind: "mode", Mode: Indexed}); err != nil {
		t.Fatal(err)
	}
	if s.Info().Document.Canvas.Mode != Indexed {
		t.Fatal("mode")
	}
}
func TestMalformedTagDoesNotPoisonPendingEdit(t *testing.T) {
	s, _ := sessionForTest(t)
	e, err := s.Document.Begin(s.Document.Revision(), "tags")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.SetTag(Tag{Name: "bad", From: "missing", To: s.Frame, Direction: Forward}); err == nil {
		t.Fatal("accepted invalid tag")
	}
	if err = e.DeleteFrame(s.Frame); err == nil {
		t.Fatal("deleted last frame")
	}
	if len(e.Info().Tags) != 0 {
		t.Fatal("invalid tag retained")
	}
	e.Cancel()
}

func TestSurfaceChannelsSurviveAlbedoModeAndPaletteChanges(t *testing.T) {
	s, _ := sessionForTest(t)
	ctx := context.Background()
	normal := Color{128, 129, 255, 255}
	if err := s.Select("", "", Normal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(ctx, Operation{Kind: "stroke", Points: []image.Point{{3, 4}}, Brush: Brush{Size: 1, Paint: Paint{Color: normal}}}); err != nil {
		t.Fatal(err)
	}
	normalCel := s.ActiveCel()
	if err := s.Select("", "", Albedo); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(ctx, Operation{Kind: "mode", Mode: Indexed}); err != nil {
		t.Fatal(err)
	}
	c, err := s.Document.Pixel(normalCel, 3, 4)
	if err != nil || c != normal {
		t.Fatalf("normal corrupted by indexed albedo: %v %v", c, err)
	}
	order := make([]int, len(s.Document.Info().Palette))
	for i := range order {
		order[i] = i
	}
	order[1], order[2] = order[2], order[1]
	if _, err = s.Execute(ctx, Operation{Kind: "palette-reorder", PaletteOrder: order}); err != nil {
		t.Fatal(err)
	}
	c, err = s.Document.Pixel(normalCel, 3, 4)
	if err != nil || c != normal {
		t.Fatal("normal corrupted by palette remap")
	}
	data, err := s.Document.Encode()
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decode(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	c, err = d.Pixel(normalCel, 3, 4)
	if err != nil || c != normal {
		t.Fatalf("normal round trip: %v %v", c, err)
	}
	im, err := d.Render(s.Frame, RenderOptions{Channel: Normal})
	if err != nil {
		t.Fatal(err)
	}
	if got := im.NRGBAAt(3, 4); got.R != 128 || got.G != 129 || got.B != 255 {
		t.Fatal("normal render")
	}
}
