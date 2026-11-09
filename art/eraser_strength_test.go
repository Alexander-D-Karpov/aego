package art

import (
	"context"
	"image"
	"testing"
)

// Deferred tests: these are not evidence of a passing build or UI acceptance.
func TestSoftEraserStrengthKeepsUndoAndLegacyDefault(t *testing.T) {
	s, _ := sessionForTest(t)
	defer s.Document.Close()
	point := image.Pt(4, 5)
	if _, err := s.Execute(context.Background(), Operation{Kind: "stroke", Points: []image.Point{point}, Brush: brush(red)}); err != nil {
		t.Fatal(err)
	}
	cel := s.ActiveCel()
	if err := s.BeginGesture("eraser", point, Brush{Size: 1, EraseStrength: 128}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.MoveGesture([]image.Point{point}); err != nil {
		t.Fatal(err)
	}
	if err := s.EndGesture(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 4, 5, Color{255, 0, 0, 127})
	if s.Info().Undo != 2 {
		t.Fatal("one erase gesture must be one history entry")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 4, 5, red)
	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 4, 5, Color{255, 0, 0, 127})
	if err := s.BeginGesture("eraser", point, Brush{Size: 1}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.EndGesture(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 4, 5, Color{})
}

func TestSmudgeUsesPreviousDabAndOneUndo(t *testing.T) {
	s, _ := sessionForTest(t)
	defer s.Document.Close()
	for _, v := range []struct {
		point image.Point
		color Color
	}{{image.Pt(1, 1), red}, {image.Pt(2, 1), blue}} {
		if _, err := s.Execute(context.Background(), Operation{Kind: "stroke", Points: []image.Point{v.point}, Brush: brush(v.color)}); err != nil {
			t.Fatal(err)
		}
	}
	cel, before := s.ActiveCel(), s.Info().Undo
	if err := s.BeginGesture("eraser", image.Pt(1, 1), Brush{Size: 1, EraseStrength: 128, Smudge: true}, false); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 2, 1, blue)
	if err := s.MoveGesture([]image.Point{{2, 1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.EndGesture(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 2, 1, Color{128, 0, 127, 255})
	if s.Info().Undo != before+1 {
		t.Fatal("smudge must publish exactly one history entry")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 2, 1, blue)
	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	pixel(t, s.Document, cel, 2, 1, Color{128, 0, 127, 255})
}

func TestSoftEraserRespectsAlphaLockAndIndexedPalette(t *testing.T) {
	d, layer, _, cel := doc(t, Indexed)
	defer d.Close()
	e := edit(t, d)
	if err := e.SetPixel(cel, 2, 2, Paint{UseIndex: true, Index: 6}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	before := d.Info()
	e = edit(t, d)
	l := e.Info().Layers[0]
	if l.ID != layer {
		t.Fatal("fixture layer changed")
	}
	l.AlphaLock = true
	if err := e.SetLayer(l); err != nil {
		t.Fatal(err)
	}
	g, err := e.Stroke(cel, Brush{Size: 1, Erase: true, EraseStrength: 128}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Point(image.Pt(2, 2)); err != nil {
		t.Fatal(err)
	}
	if got, err := e.Pixel(cel, 2, 2); err != nil || got != before.Palette[6].Color {
		t.Fatalf("alpha lock did not protect indexed pixel: %+v, %v", got, err)
	}
	if len(e.Info().Palette) != len(before.Palette) {
		t.Fatal("soft erasing changed the indexed palette")
	}
	e.Cancel()
}
