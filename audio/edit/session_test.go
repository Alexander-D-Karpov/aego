package audioedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"testing"
)

const testSourceDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func testSource(frames int64) Source {
	return Source{AssetID: "clip-1", Path: "assets/audio/glorious-morning.ogg", SHA256: testSourceDigest, Codec: "ogg", SampleRate: 48000, Channels: 2, Frames: frames}
}

func TestSessionUndoIsCleanAndRevisionSafe(t *testing.T) {
	d, err := New(testSource(48000))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(d, 2)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Mutate(0, func(d *Document) error { SetLoop(d, &Range{1000, 10000}); return nil })
	if err != nil || r != 1 || !s.Dirty() {
		t.Fatalf("mutation rev=%d err=%v dirty=%t", r, err, s.Dirty())
	}
	if _, err := s.Mutate(0, func(d *Document) error { return nil }); err == nil {
		t.Fatal("stale mutation accepted")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.Dirty() {
		t.Fatal("undo must restore clean content")
	}
	if err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if !s.Dirty() {
		t.Fatal("redo must restore dirty content")
	}
	if err := s.MarkSaved(1); err == nil {
		t.Fatal("stale save accepted")
	}
	if err := s.MarkSaved(s.Snapshot().Revision); err != nil {
		t.Fatal(err)
	}
	if s.Dirty() {
		t.Fatal("saved still dirty")
	}
}

func TestLinksCopyAndValidation(t *testing.T) {
	d, _ := New(testSource(48000))
	s, _ := Open(d, 16)
	_, err := s.Mutate(0, func(d *Document) error {
		SetLoop(d, &Range{100, 200})
		AddMarker(d, Marker{ID: "intro", Frame: 120})
		AddOperation(d, Operation{Kind: FadeIn, Region: Range{0, 1000}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	copy := s.Snapshot()
	copy.Markers[0].Label = "changed"
	copy.Loop.End = 300
	if s.Snapshot().Loop.End != 200 || s.Snapshot().Markers[0].Label != "" {
		t.Fatal("snapshot shares mutable slices/pointers")
	}
	b, err := s.Snapshot().Encode()
	if err != nil {
		t.Fatal(err)
	}
	re, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(re, s.Snapshot()) {
		t.Fatal("document round-trip changed")
	}
	_, err = s.Mutate(s.Snapshot().Revision, func(d *Document) error { SetTrim(d, Range{500, 20000}); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Loop != nil || len(s.Snapshot().Markers) != 0 {
		t.Fatal("trim left invalid associated references")
	}
}

type fakePCM struct {
	channels int
	samples  []float32
	err      error
}

func (r fakePCM) ReadFrames(ctx context.Context, start int64, n int) ([]float32, error) {
	if r.err != nil {
		return nil, r.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := make([]float32, n*r.channels)
	copy(out, r.samples[int(start)*r.channels:(int(start)+n)*r.channels])
	return out, nil
}
func TestPeakPyramidAndVisible(t *testing.T) {
	samples := make([]float32, 8193*2)
	for i := 0; i < 8193; i++ {
		samples[i*2] = .5
		samples[i*2+1] = -.25
	}
	samples[0] = 1
	p, err := BuildPyramid(context.Background(), fakePCM{channels: 2, samples: samples}, 8193, 2, 256)
	if err != nil {
		t.Fatal(err)
	}
	if p.Channels != 2 || p.Levels[0].Samples[0].Max != 1 || p.Levels[0].Samples[0].Clipped != 1 {
		t.Fatalf("wrong peaks %+v", p.Levels[0].Samples[0])
	}
	if last := p.Levels[0].Samples[len(p.Levels[0].Samples)-1]; last.Frames != 1 {
		t.Fatalf("last bucket=%+v", last)
	}
	if _, _, _, err := p.Visible(0, 8193, 100); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := p.Visible(0, 8193, 0); err == nil {
		t.Fatal("bad viewport accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = BuildPyramid(ctx, fakePCM{channels: 2, samples: samples}, 8193, 2, 256)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	_, err = BuildPyramid(context.Background(), fakePCM{channels: 2, err: errors.New("decode")}, 8193, 2, 256)
	if err == nil {
		t.Fatal("decoder error was ignored")
	}
	samples[1] = float32(math.NaN())
	if _, err = BuildPyramid(context.Background(), fakePCM{channels: 2, samples: samples}, 8193, 2, 256); err == nil {
		t.Fatal("NaN passed")
	}
}
func TestSourceIsNotModified(t *testing.T) {
	raw := []byte("this is the original lossy Ogg data")
	before := sha256.Sum256(raw)
	d, _ := New(testSource(1000))
	s, _ := Open(d, 16)
	_, err := s.Mutate(0, func(d *Document) error {
		AddOperation(d, Operation{Kind: Gain, Region: Range{10, 500}, Value: .5})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Undo()
	_ = s.Redo()
	after := sha256.Sum256(raw)
	if hex.EncodeToString(before[:]) != hex.EncodeToString(after[:]) {
		t.Fatal("original source modified")
	}
}

func TestNoOpDoesNotCreateHistoryAndSettersOwnRanges(t *testing.T) {
	d, err := New(testSource(48000))
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(d, 16)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := s.Mutate(0, func(d *Document) error { return nil }); err != nil || r != 0 || s.CanUndo() || s.Dirty() {
		t.Fatalf("no-op created a revision: rev=%d err=%v", r, err)
	}
	loop := Range{Start: 100, End: 200}
	r, err := s.Mutate(0, func(d *Document) error { SetLoop(d, &loop); return nil })
	if err != nil || r != 1 {
		t.Fatalf("loop: %d %v", r, err)
	}
	loop.End = 300
	if got := s.Snapshot().Loop.End; got != 200 {
		t.Fatalf("external pointer mutated document: %d", got)
	}
}
