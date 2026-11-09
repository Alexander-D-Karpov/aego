package art

import (
	"context"
	"image"
	"testing"
)

func TestCapturedStampAndPatternShareGestureHistory(t *testing.T) {
	s, _ := sessionForTest(t)
	defer s.Document.Close()
	for _, v := range []struct {
		p image.Point
		c Color
	}{{image.Pt(1, 1), red}, {image.Pt(2, 1), blue}, {image.Pt(1, 2), blue}} {
		if _, e := s.Execute(context.Background(), Operation{Kind: "stroke", Points: []image.Point{v.p}, Brush: brush(v.c)}); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.SetSelection("rectangle", []image.Point{{1, 1}, {3, 2}}, 0); e != nil {
		t.Fatal(e)
	}
	before := s.Info()
	if e := s.CaptureBrush(); e != nil {
		t.Fatal(e)
	}
	if got := s.Info(); got.Undo != before.Undo || got.Document.Revision != before.Document.Revision || got.BrushWidth != 3 || got.BrushHeight != 2 {
		t.Fatalf("capture changed document or dimensions: %+v", got)
	}
	if e := s.SetSelection("none", nil, 0); e != nil {
		t.Fatal(e)
	}
	// Capture is independent of later source edits and of the paste clipboard.
	s.Clipboard = &Clipboard{Width: 1, Height: 1, Pixels: []Color{red}}
	if e := s.BeginGesture("pencil", image.Pt(10, 10), Brush{Source: "stamp", Size: 1, PixelPerfect: true}, false); e != nil {
		t.Fatal(e)
	}
	if e := s.EndGesture(); e != nil {
		t.Fatal(e)
	}
	cel := s.ActiveCel()
	pixel(t, s.Document, cel, 9, 9, red)
	pixel(t, s.Document, cel, 10, 9, blue)
	pixel(t, s.Document, cel, 11, 9, Color{})
	pixel(t, s.Document, cel, 9, 10, blue)
	if got := s.Info().Undo; got != before.Undo+1 {
		t.Fatal("one stamp must be one Undo", got)
	}
	if e := s.Undo(); e != nil {
		t.Fatal(e)
	}
	pixel(t, s.Document, cel, 9, 9, Color{})
	if e := s.Redo(); e != nil {
		t.Fatal(e)
	}
	pixel(t, s.Document, cel, 9, 9, red)
	if e := s.BeginGesture("pencil", image.Pt(10, 10), Brush{Source: "stamp", Size: 1}, false); e != nil {
		t.Fatal(e)
	}
	if e := s.MoveGesture([]image.Point{{11, 10}}); e != nil {
		t.Fatal(e)
	}
	if e := s.EndGesture(); e != nil {
		t.Fatal(e)
	}
	// Moving a colored stamp replaces overlapping pixels with the latest dab.
	pixel(t, s.Document, cel, 10, 9, red)
	pixel(t, s.Document, cel, 11, 9, blue)
	if e := s.SetSelection("rectangle", []image.Point{{20, 20}, {23, 23}}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.BeginGesture("pencil", image.Pt(20, 20), Brush{Source: "pattern", Size: 8}, false); e != nil {
		t.Fatal(e)
	}
	if e := s.EndGesture(); e != nil {
		t.Fatal(e)
	}
	pixel(t, s.Document, cel, 21, 20, red)
	pixel(t, s.Document, cel, 22, 20, blue)
	pixel(t, s.Document, cel, 19, 20, Color{})
	s.ClearBrush()
	if e := s.BeginGesture("pencil", image.Pt(5, 5), Brush{Source: "stamp", Size: 1}, false); e == nil {
		t.Fatal("cleared brush silently painted with a solid color")
	}
}

func TestCaptureBrushRequiresBoundedSelection(t *testing.T) {
	s, _ := sessionForTest(t)
	defer s.Document.Close()
	if e := s.CaptureBrush(); e == nil {
		t.Fatal("empty selection accepted")
	}
	if e := s.SetSelection("all", nil, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.BeginGesture("pencil", image.Pt(2, 2), brush(red), false); e != nil {
		t.Fatal(e)
	}
	if e := s.CaptureBrush(); e == nil {
		t.Fatal("unconfirmed gesture captured")
	}
	s.CancelGesture()
}
