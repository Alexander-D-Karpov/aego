package art

import (
	"context"
	"errors"
	"image"
	"testing"
)

func TestSamplePaintKeepsIndexedIdentityAndDoesNotEdit(t *testing.T) {
	d, _, _, cel := doc(t, Indexed)
	e := edit(t, d)
	palette := d.Info().Palette
	if err := e.SetPaletteEntry(4, palette[3]); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(cel, 2, 3, Paint{UseIndex: true, Index: 4}, nil, false); err != nil {
		t.Fatal(err)
	}
	done(t, e)
	s, err := NewSession(d, NewJournal(Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	before := d.Revision()
	got, err := s.SamplePaint(image.Pt(2, 3))
	if err != nil || !got.UseIndex || got.Index != 4 || got.Color != palette[3].Color {
		t.Fatalf("sampled %+v, %v; expected the actual index 4 despite duplicate colors", got, err)
	}
	if d.Revision() != before || s.Info().Dirty || s.Info().Undo != 0 {
		t.Fatal("eyedropper changed the document or history")
	}
	if _, err := s.SamplePaint(image.Pt(-1, 0)); err == nil {
		t.Fatal("sampled outside the canvas")
	}
	if err := s.BeginGesture("pencil", image.Pt(1, 1), Brush{Size: 1, Paint: Paint{UseIndex: true, Index: 4}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SamplePaint(image.Pt(1, 1)); err == nil {
		t.Fatal("sampled during an unfinished gesture")
	}
	s.CancelGesture()
	if _, err := s.Execute(context.Background(), Operation{Kind: "add-frame", DurationMS: 100}); err != nil {
		t.Fatal(err)
	}
	got, err = s.SamplePaint(image.Pt(2, 3))
	if err != nil || !got.UseIndex || got.Index != 0 || got.Color.A != 0 {
		t.Fatalf("empty frame did not sample transparent index zero: %+v, %v", got, err)
	}
	if err := s.Select("", "", Normal); err != nil {
		t.Fatal(err)
	}
	got, err = s.SamplePaint(image.Pt(2, 3))
	if err != nil || got.UseIndex || got.Color != (Color{}) {
		t.Fatalf("empty normal channel incorrectly used albedo's indexed mode: %+v, %v", got, err)
	}
	if _, err := s.Execute(context.Background(), Operation{Kind: "stroke", Points: []image.Point{{2, 3}}, Brush: Brush{Size: 1, Paint: Paint{Color: Color{127, 128, 255, 255}}}}); err != nil {
		t.Fatal(err)
	}
	got, err = s.SamplePaint(image.Pt(2, 3))
	if err != nil || got.UseIndex || got.Color != (Color{127, 128, 255, 255}) {
		t.Fatalf("wrong normal-channel sample: %+v, %v", got, err)
	}
	d.Close()
	if _, err := d.SamplePaint(cel, Albedo, 2, 3); !errors.Is(err, ErrClosed) {
		t.Fatalf("sampling a closed document: %v", err)
	}
}
