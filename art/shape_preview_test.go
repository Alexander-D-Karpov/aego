package art

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"
)

func TestMultiPointShapePreviewCommitCancelAndConflict(t *testing.T) {
	for _, tool := range []string{"curve", "polygon"} {
		t.Run(tool, func(t *testing.T) {
			s, history := sessionForTest(t)
			points := []image.Point{{2, 3}, {7, 19}, {19, 4}, {27, 26}}
			brush := Brush{Size: 1, Paint: Paint{Color: Color{235, 92, 13, 255}}}
			revision := s.Document.Revision()
			if err := s.BeginGesture(tool, points[0], brush, false); err != nil {
				t.Fatal(err)
			}
			if err := s.PreviewGesturePoints(points); err != nil {
				t.Fatal(err)
			}
			preview, err := s.Preview("", false, false)
			if err != nil || preview.NRGBAAt(2, 3).A != 255 {
				t.Fatalf("shape preview missing: %v", err)
			}
			if s.Document.Revision() != revision || s.Info().Undo != 0 || s.Info().Dirty {
				t.Fatal("preview modified canonical state or history")
			}
			if err := s.EndGesture(); err != nil {
				t.Fatal(err)
			}
			committed, err := s.Preview("", false, false)
			if err != nil || !bytes.Equal(preview.Pix, committed.Pix) || s.Info().Undo != 1 {
				t.Fatal("shape commit differs from its preview or creates multiple undo entries")
			}
			if err := s.Undo(); err != nil {
				t.Fatal(err)
			}
			if err := s.BeginGesture(tool, points[0], brush, false); err != nil {
				t.Fatal(err)
			}
			if err := s.PreviewGesturePoints(points); err != nil {
				t.Fatal(err)
			}
			s.CancelGesture()
			cancelled, err := s.Preview("", false, false)
			if err != nil || cancelled.NRGBAAt(2, 3).A != 0 || s.Info().Undo != 0 {
				t.Fatal("cancelled shape leaked pixels or history")
			}
			if err := s.BeginGesture(tool, points[0], brush, false); err != nil {
				t.Fatal(err)
			}
			change, _, err := s.Document.Propose(context.Background(), s.Document.Revision(), "concurrent duration", []Operation{{Kind: "duration", Frame: s.Frame, DurationMS: 175}})
			if err != nil {
				t.Fatal(err)
			}
			if err = history.Exec(change); err != nil {
				t.Fatal(err)
			}
			if err = s.PreviewGesturePoints(points); !errors.Is(err, ErrConflict) || s.Info().Gesture {
				t.Fatalf("stale shape survives: %v", err)
			}
		})
	}
}
