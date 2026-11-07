package art

import (
	"bytes"
	"context"
	"errors"
	"image"
	"testing"
)

func assertCachedPreview(t *testing.T, s *Session, frame ID, refs bool) {
	t.Helper()
	if frame == "" {
		frame = s.Frame
	}
	actual, err := s.Preview(frame, false, refs)
	if err != nil {
		t.Fatal(err)
	}
	opts := RenderOptions{Channel: s.Channel, References: refs}
	var expected *image.NRGBA
	if s.draft != nil && frame == s.Frame {
		expected, err = s.draft.Render(frame, opts)
	} else {
		expected, err = s.Document.Render(frame, opts)
	}
	if err != nil || !bytes.Equal(actual.Pix, expected.Pix) {
		t.Fatalf("incremental pixels differ from full composition: %v", err)
	}
}
func cacheSession(t *testing.T) *Session {
	t.Helper()
	d, err := New(64, 64, RGBA, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(d, NewJournal(Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestPreviewCacheReusesUnchangedFramesAndOwnsReturnedPixels(t *testing.T) {
	s := cacheSession(t)
	assertCachedPreview(t, s, "", false)
	count := s.preview.composites
	im, err := s.Preview("", false, false)
	if err != nil {
		t.Fatal(err)
	}
	im.Pix[0] = 255
	assertCachedPreview(t, s, "", false)
	if s.preview.composites != count {
		t.Fatal("unchanged preview was recomposited")
	}
	if _, err = s.Execute(context.Background(), Operation{Kind: "duration", DurationMS: 177}); err != nil {
		t.Fatal(err)
	}
	assertCachedPreview(t, s, "", false)
	if s.preview.composites != count {
		t.Fatal("duration change decoded/composited pixels")
	}
	pixels := s.preview.compositedPixels
	if _, err = s.Execute(context.Background(), Operation{Kind: "stroke", Points: []image.Point{{3, 5}}, Brush: brush(red)}); err != nil {
		t.Fatal(err)
	}
	assertCachedPreview(t, s, "", false)
	if s.preview.compositedPixels-pixels > TileSize*TileSize {
		t.Fatal("a point invalidated the entire canvas")
	}
	s.Document.Close()
	if _, err = s.Preview("", false, false); !errors.Is(err, ErrClosed) || len(s.preview.entries) != 0 {
		t.Fatalf("closed: %v", err)
	}
}
func TestPreviewCacheDraftMutableTilesShapesAndCancel(t *testing.T) {
	s := cacheSession(t)
	for _, tool := range []string{"pencil", "rectangle", "ellipse", "gradient"} {
		assertCachedPreview(t, s, "", false)
		if err := s.BeginGesture(tool, image.Pt(2, 2), brush(red), true); err != nil {
			t.Fatal(err)
		}
		for _, point := range []image.Point{{5, 5}, {45, 40}, {9, 8}, {20, 12}} {
			if err := s.MoveGesture([]image.Point{point}); err != nil {
				t.Fatal(err)
			}
			assertCachedPreview(t, s, "", false)
			assertCachedPreview(t, s, "", true)
		}
		s.CancelGesture()
		assertCachedPreview(t, s, "", false)
		assertCachedPreview(t, s, "", true)
		if err := s.BeginGesture(tool, image.Pt(2, 2), brush(blue), true); err != nil {
			t.Fatal(err)
		}
		assertCachedPreview(t, s, "", false)
		if err := s.MoveGesture([]image.Point{{30, 16}}); err != nil {
			t.Fatal(err)
		}
		// Commit without rendering the final pointer event first.
		if err := s.EndGesture(); err != nil {
			t.Fatal(err)
		}
		assertCachedPreview(t, s, "", false)
		if err := s.Undo(); err != nil {
			t.Fatal(err)
		}
		assertCachedPreview(t, s, "", false)
		if err := s.Redo(); err != nil {
			t.Fatal(err)
		}
		assertCachedPreview(t, s, "", false)
	}
}
func TestPreviewCacheLinkedFramesModesPaletteAndDeletion(t *testing.T) {
	s := cacheSession(t)
	ctx := context.Background()
	for _, op := range []Operation{
		{Kind: "rectangle", Points: []image.Point{{3, 3}, {24, 20}}, Brush: brush(red), Filled: true},
		{Kind: "duplicate-frame", Linked: true},
		{Kind: "duplicate-frame", Linked: true},
		{Kind: "stroke", Points: []image.Point{{5, 5}}, Brush: brush(blue)},
		{Kind: "unlink"},
		{Kind: "flip-x"},
		{Kind: "mode", Mode: Indexed},
		{Kind: "palette-extract", ColorLimit: 3, Quantize: true},
		{Kind: "mode", Mode: Grayscale},
		{Kind: "delete-frame"},
	} {
		if _, err := s.Execute(ctx, op); err != nil {
			t.Fatalf("%s: %v", op.Kind, err)
		}
		for _, f := range s.Document.Info().Frames {
			assertCachedPreview(t, s, f.ID, false)
		}
		if err := s.Undo(); err != nil {
			t.Fatal(err)
		}
		for _, f := range s.Document.Info().Frames {
			assertCachedPreview(t, s, f.ID, false)
		}
		if err := s.Redo(); err != nil {
			t.Fatal(err)
		}
		for _, f := range s.Document.Info().Frames {
			assertCachedPreview(t, s, f.ID, false)
		}
	}
}
func TestPreviewCacheHasBoundedRetention(t *testing.T) {
	d, _, _ := stressDocument(t)
	s, err := NewSession(d, NewJournal(Limits{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range d.Info().Frames {
		assertCachedPreview(t, s, f.ID, false)
	}
	if len(s.preview.entries) > previewCacheEntries {
		t.Fatal("unbounded frame cache")
	}
	var cost int64
	for _, e := range s.preview.entries {
		cost += e.bytes
	}
	if cost > previewCacheBytes {
		t.Fatal("cache exceeded memory budget")
	}
	s.ClearPreview()
	if len(s.preview.entries) != 0 {
		t.Fatal("cache retained closed presentation")
	}
}
func BenchmarkSessionPreviewStressFrame16x64(b *testing.B) {
	d, frame, _ := stressDocument(b)
	s, err := NewSession(d, NewJournal(Limits{}))
	if err != nil {
		b.Fatal(err)
	}
	if _, err = s.Preview(frame, false, false); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = s.Preview(frame, false, false); err != nil {
			b.Fatal(err)
		}
	}
}
