package audioedit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func kernelSource(frames int64, channels int) Source {
	return Source{AssetID: "synthetic", SHA256: strings.Repeat("a", 64), Codec: "wav", SampleRate: 48000, Channels: channels, Frames: frames}
}

type failingKernelReader struct{ calls atomic.Int32 }

func (r *failingKernelReader) ReadFrames(context.Context, int64, int) ([]float32, error) {
	r.calls.Add(1)
	return nil, errors.New("synthetic decoder failed")
}

// A synthetic float PCM fixture opened read-only for each bounded request.
// This is not an OGG/WAV decoder or a test of OS-level source guards.
type fileKernelReader struct{ path string }

func (r fileKernelReader) ReadFrames(ctx context.Context, start int64, frames int) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(r.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data := make([]byte, frames*4)
	if _, err := f.ReadAt(data, start*4); err != nil {
		return nil, err
	}
	pcm := make([]float32, frames)
	for i := range pcm {
		pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return pcm, nil
}

func TestKernelNormalizeMeasuresEffectiveUnclippedOperations(t *testing.T) {
	d, err := New(kernelSource(8, 1))
	if err != nil {
		t.Fatal(err)
	}
	raw := []float32{.8, .8, .8, .8, .8, .8, .8, .8}
	r := fakePCM{channels: 1, samples: raw}
	for _, tc := range []struct{ gain, peak float64 }{{.5, .4}, {2, 1.6}} {
		d.Operations = []Operation{{Kind: Gain, Region: Range{0, 8}, Value: tc.gain}}
		peak, err := MeasurePeak(context.Background(), r, d, Range{0, 8})
		if err != nil || math.Abs(peak-tc.peak) > 1e-6 {
			t.Fatalf("gain %.2f: measured %v, want %v: %v", tc.gain, peak, tc.peak, err)
		}
		mult, err := NormalizeMultiplier(peak, .8)
		if err != nil {
			t.Fatal(err)
		}
		d.Operations = append(d.Operations, Operation{Kind: Normalize, Region: Range{0, 8}, Value: mult})
		out, err := RenderWindow(context.Background(), r, d, Range{0, 8}, 8)
		if err != nil || math.Abs(float64(out[0])-.8) > 1e-6 {
			t.Fatalf("gain %.2f: normalized output %v: %v", tc.gain, out, err)
		}
	}
}

func TestKernelPyramidFiniteOutOfRangeSamples(t *testing.T) {
	for _, value := range []float32{2, -2, math.MaxFloat32, -math.MaxFloat32} {
		samples := make([]float32, 256)
		for i := range samples {
			samples[i] = value
		}
		p, err := BuildPyramid(context.Background(), fakePCM{channels: 1, samples: samples}, 256, 1, 32)
		if err != nil {
			t.Fatalf("value %g: %v", value, err)
		}
		for _, lvl := range p.Levels {
			for _, peak := range lvl.Samples {
				if peak.Min != value || peak.Max != value || peak.RMS != float32(math.Abs(float64(value))) {
					t.Fatalf("value %g: wrong peak %+v", value, peak)
				}
			}
		}
	}
}

func TestKernelPyramidOverflowFailsWithoutPanic(t *testing.T) {
	defer func() {
		if v := recover(); v != nil {
			t.Errorf("overflow panicked instead of returning an error: %v", v)
		}
	}()
	r := &failingKernelReader{}
	if _, err := BuildPyramid(context.Background(), r, math.MaxInt64, 1, 256); err == nil || r.calls.Load() != 0 {
		t.Fatalf("huge pyramid must be rejected before reading; err %v, reads %d", err, r.calls.Load())
	}
}

func TestKernelWAVOverflowRejectedBeforeWrite(t *testing.T) {
	d, err := New(kernelSource(math.MaxInt64/8, 32))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	r := &failingKernelReader{}
	err = ExportWAV16(context.Background(), &output, r, d, Range{0, d.Source.Frames})
	if err == nil || output.Len() != 0 || r.calls.Load() != 0 {
		t.Fatalf("oversized WAV: err %v, wrote %d bytes, reads %d", err, output.Len(), r.calls.Load())
	}
}

func TestKernelPyramidBudgetIncludesChannelsAndLevels(t *testing.T) {
	r := &failingKernelReader{}
	_, err := BuildPyramid(context.Background(), r, 1_600_000, 32, 32)
	if err == nil || r.calls.Load() != 0 {
		t.Fatalf("oversized pyramid must fail before allocation/read; err %v, reads %d", err, r.calls.Load())
	}
}

func TestKernelRenderWindowHasHardMemoryBudget(t *testing.T) {
	d, err := New(kernelSource(300_000, 32))
	if err != nil {
		t.Fatal(err)
	}
	r := &failingKernelReader{}
	if _, err := RenderWindow(context.Background(), r, d, Range{0, 300_000}, 300_000); err == nil || r.calls.Load() != 0 {
		t.Fatalf("oversized render must fail before reading; err %v, reads %d", err, r.calls.Load())
	}
}

func TestKernelMalformedPyramidDoesNotPanic(t *testing.T) {
	for _, p := range []Pyramid{
		{TotalFrames: 1, Levels: []Level{{FramesPerBucket: 1}}},
		{Channels: 1, TotalFrames: 1, Levels: []Level{{FramesPerBucket: 0, Samples: []Peak{{}}}}},
		{Channels: 2, TotalFrames: 1, Levels: []Level{{FramesPerBucket: 1, Samples: []Peak{{}}}}},
	} {
		func() {
			defer func() {
				if v := recover(); v != nil {
					t.Errorf("malformed pyramid panicked: %v", v)
				}
			}()
			if _, _, _, err := p.Visible(0, 1, 1); err == nil {
				t.Error("malformed pyramid accepted")
			}
		}()
	}
}

func TestKernelRejectsNonFinitePCMAndBadInputs(t *testing.T) {
	for _, sample := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		samples := make([]float32, 64)
		samples[1] = sample // A non-first channel must not hide corrupt PCM.
		r := fakePCM{channels: 2, samples: samples}
		d, err := New(kernelSource(32, 2))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = BuildPyramid(context.Background(), r, 32, 2, 32); err == nil {
			t.Fatal("peak accepted non-finite PCM")
		}
		if _, err = RenderWindow(context.Background(), r, d, Range{0, 32}, 32); err == nil {
			t.Fatal("render accepted non-finite PCM")
		}
		if _, err = MeasurePeak(context.Background(), r, d, Range{0, 32}); err == nil {
			t.Fatal("normalization accepted non-finite PCM")
		}
		if _, err = NearestZeroCrossing(context.Background(), r, d.Source, 0, 1); err == nil {
			t.Fatal("zero snap accepted non-finite PCM in another channel")
		}
	}
	d, _ := New(kernelSource(8, 1))
	d.Trim = Range{2, 6}
	r := fakePCM{channels: 1, samples: make([]float32, 8)}
	if _, err := MeasurePeak(context.Background(), r, d, Range{0, 4}); err == nil {
		t.Fatal("normalization outside trim was accepted")
	}
	for _, kind := range []OperationKind{Gain, Normalize, FadeIn, FadeOut, Silence, Reverse} {
		op := Operation{Kind: kind, Region: Range{0, 4}, Value: math.NaN()}
		if err := op.Validate(8); err == nil {
			t.Fatalf("%s accepted non-finite value", kind)
		}
	}
}

type shortKernelWriter struct{ calls int }

func (w *shortKernelWriter) Write(p []byte) (int, error) {
	w.calls++
	return len(p) - 1, nil
}

func TestKernelWAVReportsShortWrite(t *testing.T) {
	d, _ := New(kernelSource(8, 1))
	w := &shortKernelWriter{}
	err := ExportWAV16(context.Background(), w, fakePCM{channels: 1, samples: make([]float32, 8)}, d, Range{0, 8})
	if !errors.Is(err, io.ErrShortWrite) || w.calls != 1 {
		t.Fatalf("short write: err %v, calls %d", err, w.calls)
	}
}

func TestKernelSessionCannotMutateSourceOrRetainAliases(t *testing.T) {
	d, _ := New(kernelSource(8, 1))
	s, err := Open(d, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Mutate(0, func(next *Document) error { next.Source.SHA256 = strings.Repeat("b", 64); return nil }); err == nil {
		t.Fatal("generic mutation silently replaced the source identity")
	}
	var retained *Document
	_, err = s.Mutate(0, func(next *Document) error {
		SetLoop(next, &Range{1, 7})
		AddOperation(next, Operation{Kind: Gain, Region: Range{0, 8}, Value: .5})
		retained = next
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	retained.Loop.End = 5
	retained.Operations[0].Value = 2
	got := s.Snapshot()
	if got.Loop.End != 7 || got.Operations[0].Value != .5 {
		t.Fatalf("callback retained mutable access to session: %+v", got)
	}
}

func TestKernelNonDestructiveOperationsUndoRedo(t *testing.T) {
	raw := []float32{-.8, -.6, -.4, -.2, .2, .4, .6, .8}
	encoded := make([]byte, len(raw)*4)
	for i, sample := range raw {
		binary.LittleEndian.PutUint32(encoded[i*4:], math.Float32bits(sample))
	}
	before := sha256.Sum256(encoded)
	sourcePath := filepath.Join(t.TempDir(), "source.pcm")
	if err := os.WriteFile(sourcePath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	source := kernelSource(8, 1)
	source.Path, source.SHA256 = sourcePath, hex.EncodeToString(before[:])
	d, err := New(source)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(d, 4)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Mutate(0, func(next *Document) error {
		SetTrim(next, Range{1, 7})
		SetLoop(next, &Range{2, 6})
		AddOperation(next, Operation{Kind: Reverse, Region: Range{0, 8}})
		AddOperation(next, Operation{Kind: FadeIn, Region: Range{1, 7}})
		AddMarker(next, Marker{ID: "m", Frame: 3})
		return nil
	})
	if err != nil || !s.Dirty() {
		t.Fatalf("edit: dirty %v, err %v", s.Dirty(), err)
	}
	edited := s.Snapshot()
	reader := fileKernelReader{path: sourcePath}
	if _, err := RenderWindow(context.Background(), reader, edited, Range{1, 7}, 6); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := ExportWAV16(context.Background(), &output, reader, edited, Range{1, 7}); err != nil {
		t.Fatal(err)
	}
	if err := s.Undo(); err != nil || s.Dirty() {
		t.Fatalf("undo: dirty %v, err %v", s.Dirty(), err)
	}
	if err := s.Redo(); err != nil || !s.Dirty() {
		t.Fatalf("redo: dirty %v, err %v", s.Dirty(), err)
	}
	again := s.Snapshot()
	again.Revision, edited.Revision = 0, 0
	if !reflect.DeepEqual(again, edited) {
		t.Fatal("redo changed metadata")
	}
	saved, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.Equal(saved, encoded) || sha256.Sum256(saved) != before {
		t.Fatalf("synthetic source changed: %v", err)
	}
}

func TestKernelSessionConcurrentMutationsAndSnapshots(t *testing.T) {
	d, _ := New(kernelSource(8, 1))
	s, err := Open(d, 64)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 40; n++ {
				rev := s.Snapshot().Revision
				if _, err := s.Mutate(rev, func(d *Document) error { d.BPM++; return nil }); err == nil {
					successes.Add(1)
				}
				_ = s.Dirty()
				_ = s.CanUndo()
				_ = s.CanRedo()
			}
		}()
	}
	wg.Wait()
	got := s.Snapshot()
	if successes.Load() == 0 || got.BPM != float64(successes.Load()) || got.Revision != uint64(successes.Load()) {
		t.Fatalf("concurrent result: successes %d, doc %+v", successes.Load(), got)
	}
}
