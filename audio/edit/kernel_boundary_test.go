package audioedit

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestKernelReferenceOperationOrderAndWindows(t *testing.T) {
	random := rand.New(rand.NewSource(20261110))
	const frames, channels = 65, 2
	for trial := 0; trial < 120; trial++ {
		d, err := New(kernelSource(frames, channels))
		if err != nil {
			t.Fatal(err)
		}
		samples := make([]float32, frames*channels)
		reference := make([]float64, len(samples))
		for i := range samples {
			samples[i] = float32(random.Float64()*1.6 - .8)
			reference[i] = float64(samples[i])
		}
		for j := 0; j < 12; j++ {
			start := random.Intn(frames)
			end := start + 1 + random.Intn(frames-start)
			op := Operation{Region: Range{int64(start), int64(end)}}
			switch random.Intn(6) {
			case 0:
				op.Kind, op.Value = Gain, []float64{0, .5, 1, 1.5, 2}[random.Intn(5)]
			case 1:
				op.Kind, op.Value = Normalize, .75
			case 2:
				op.Kind = Reverse
			case 3:
				op.Kind = Silence
			case 4:
				op.Kind = FadeIn
			case 5:
				op.Kind = FadeOut
			}
			d.Operations = append(d.Operations, op)
			if op.Kind == Reverse {
				for left, right := start, end-1; left < right; left, right = left+1, right-1 {
					for c := 0; c < channels; c++ {
						reference[left*channels+c], reference[right*channels+c] = reference[right*channels+c], reference[left*channels+c]
					}
				}
				continue
			}
			for frame := start; frame < end; frame++ {
				factor := 1.0
				switch op.Kind {
				case Gain, Normalize:
					factor = op.Value
				case Silence:
					factor = 0
				case FadeIn:
					if end-start == 1 {
						factor = 0
					} else {
						factor = float64(frame-start) / float64(end-start-1)
					}
				case FadeOut:
					if end-start == 1 {
						factor = 0
					} else {
						factor = float64(end-1-frame) / float64(end-start-1)
					}
				}
				for c := 0; c < channels; c++ {
					reference[frame*channels+c] *= factor
				}
			}
		}
		for window := 0; window < 3; window++ {
			start := random.Intn(frames)
			end := start + 1 + random.Intn(frames-start)
			region := Range{int64(start), int64(end)}
			r := fakePCM{channels: channels, samples: samples}
			got, err := RenderWindow(context.Background(), r, d, region, frames)
			if err != nil {
				t.Fatalf("trial %d: %v", trial, err)
			}
			wantPeak := 0.0
			for i, v := range reference[start*channels : end*channels] {
				wantPeak = math.Max(wantPeak, math.Abs(v))
				want := math.Max(-1, math.Min(1, v))
				if math.Abs(float64(got[i])-want) > 1e-6 {
					t.Fatalf("trial %d window %d sample %d: got %g want %g", trial, window, i, got[i], want)
				}
			}
			peak, err := MeasurePeak(context.Background(), r, d, region)
			if err != nil || math.Abs(peak-wantPeak) > 1e-10 {
				t.Fatalf("trial %d window %d: peak %g want %g: %v", trial, window, peak, wantPeak, err)
			}
		}
	}
}

func TestKernelChunkBoundariesAndVeryLargeFiniteFloat(t *testing.T) {
	const frames = 17001
	d, _ := New(kernelSource(frames, 1))
	samples := make([]float32, frames)
	samples[8191], samples[8192], samples[16000] = -.5, .75, math.MaxFloat32
	d.Operations = []Operation{{Kind: Reverse, Region: Range{0, frames}}, {Kind: Gain, Region: Range{0, frames}, Value: 100}}
	r := fakePCM{channels: 1, samples: samples}
	peak, err := MeasurePeak(context.Background(), r, d, Range{0, frames})
	if err != nil || peak != float64(math.MaxFloat32)*100 {
		t.Fatalf("peak %g: %v", peak, err)
	}
	mult, err := NormalizeMultiplier(peak, 1)
	if err != nil {
		t.Fatal(err)
	}
	d.Operations = append(d.Operations, Operation{Kind: Normalize, Region: Range{0, frames}, Value: mult})
	position := int64(frames - 1 - 16000)
	out, err := RenderWindow(context.Background(), r, d, Range{position, position + 1}, 1)
	if err != nil || math.Abs(float64(out[0])-1) > 1e-6 {
		t.Fatalf("normalized float boundary %v: %v", out, err)
	}
}

func TestKernelWAVPCM16Boundaries(t *testing.T) {
	samples := []float32{-2, -1, -.5, 0, .5, 1, 2}
	d, _ := New(kernelSource(int64(len(samples)), 1))
	var output bytes.Buffer
	if err := ExportWAV16(context.Background(), &output, fakePCM{channels: 1, samples: samples}, d, Range{0, int64(len(samples))}); err != nil {
		t.Fatal(err)
	}
	raw := output.Bytes()
	if len(raw) != 44+len(samples)*2 || binary.LittleEndian.Uint32(raw[40:44]) != uint32(len(samples)*2) {
		t.Fatal("wrong WAV size")
	}
	for i, want := range []int16{-32768, -32768, -16384, 0, 16384, 32767, 32767} {
		if got := int16(binary.LittleEndian.Uint16(raw[44+i*2:])); got != want {
			t.Fatalf("sample %d: got %d, want %d", i, got, want)
		}
	}
}

func TestKernelWAVRIFFSizeBoundary(t *testing.T) {
	const largestFrames = int64((math.MaxUint32 - 36) / 2)
	for _, tc := range []struct {
		frames     int64
		wantHeader bool
	}{{largestFrames, true}, {largestFrames + 1, false}} {
		d, err := New(kernelSource(tc.frames, 1))
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		r := &failingKernelReader{}
		err = ExportWAV16(context.Background(), &output, r, d, Range{0, tc.frames})
		if err == nil || (output.Len() == 44) != tc.wantHeader {
			t.Fatalf("frames %d: err %v header bytes %d", tc.frames, err, output.Len())
		}
		if tc.wantHeader {
			if r.calls.Load() != 1 || binary.LittleEndian.Uint32(output.Bytes()[4:8]) != uint32(36+tc.frames*2) {
				t.Fatal("last valid RIFF size or decoder call is incorrect")
			}
		} else if output.Len() != 0 || r.calls.Load() != 0 {
			t.Fatal("oversized RIFF wrote or read before rejection")
		}
	}
}

type cancellingKernelReader struct{ cancel context.CancelFunc }

func (r cancellingKernelReader) ReadFrames(_ context.Context, _ int64, n int) ([]float32, error) {
	r.cancel()
	return make([]float32, n), nil
}

func TestKernelCancellationAfterDecoderReturns(t *testing.T) {
	for _, kind := range []string{"pyramid", "render", "measure", "snap", "export"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := cancellingKernelReader{cancel: cancel}
			d, _ := New(kernelSource(32, 1))
			var output bytes.Buffer
			var err error
			switch kind {
			case "pyramid":
				_, err = BuildPyramid(ctx, r, 32, 1, 32)
			case "render":
				_, err = RenderWindow(ctx, r, d, Range{0, 32}, 32)
			case "measure":
				_, err = MeasurePeak(ctx, r, d, Range{0, 32})
			case "snap":
				_, err = NearestZeroCrossing(ctx, r, d.Source, 0, 2)
			case "export":
				err = ExportWAV16(ctx, &output, r, d, Range{0, 32})
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("wanted cancellation, got %v", err)
			}
			if kind == "export" && output.Len() != 44 {
				t.Fatalf("streaming export should leave only an incomplete header, got %d bytes", output.Len())
			}
		})
	}
}

type shortKernelReader struct{}

func (shortKernelReader) ReadFrames(context.Context, int64, int) ([]float32, error) {
	return []float32{}, nil
}

func TestKernelInvalidInputsFailWithoutPanics(t *testing.T) {
	ctx := context.Background()
	var absent context.Context // Exercise the APIs' explicit nil-context rejection.
	d, _ := New(kernelSource(32, 1))
	r := fakePCM{channels: 1, samples: make([]float32, 32)}
	checks := []struct {
		name string
		call func() error
	}{
		{"nil peak context", func() error { _, e := BuildPyramid(absent, r, 32, 1, 32); return e }},
		{"nil peak reader", func() error { _, e := BuildPyramid(ctx, nil, 32, 1, 32); return e }},
		{"short peak reader", func() error { _, e := BuildPyramid(ctx, shortKernelReader{}, 32, 1, 32); return e }},
		{"negative peak frames", func() error { _, e := BuildPyramid(ctx, r, -1, 1, 32); return e }},
		{"nil render context", func() error { _, e := RenderWindow(absent, r, d, Range{0, 2}, 2); return e }},
		{"nil render reader", func() error { _, e := RenderWindow(ctx, nil, d, Range{0, 2}, 2); return e }},
		{"short render reader", func() error { _, e := RenderWindow(ctx, shortKernelReader{}, d, Range{0, 2}, 2); return e }},
		{"negative range", func() error { _, e := RenderWindow(ctx, r, d, Range{-1, 2}, 3); return e }},
		{"zero max window", func() error { _, e := RenderWindow(ctx, r, d, Range{0, 2}, 0); return e }},
		{"nil measure reader", func() error { _, e := MeasurePeak(ctx, nil, d, Range{0, 2}); return e }},
		{"nil snap reader", func() error { _, e := NearestZeroCrossing(ctx, nil, d.Source, 0, 1); return e }},
		{"nil writer", func() error { return ExportWAV16(ctx, nil, r, d, Range{0, 2}) }},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if v := recover(); v != nil {
					t.Errorf("panicked: %v", v)
				}
			}()
			if err := tc.call(); err == nil {
				t.Error("invalid input accepted")
			}
		})
	}
	position, err := NearestZeroCrossing(ctx, fakePCM{channels: 1, samples: []float32{.5, -.01}}, kernelSource(2, 1), 0, 0)
	if err != nil || position != 0 {
		t.Fatalf("zero-radius snap left its range: %d, %v", position, err)
	}
}

func TestKernelDocumentBoundsAndStrictDecode(t *testing.T) {
	for _, bad := range []string{"", "deadbeef", strings.Repeat("z", 64), strings.Repeat("a", 63)} {
		s := kernelSource(8, 1)
		s.SHA256 = bad
		if err := s.Validate(); err == nil {
			t.Fatalf("invalid digest %q accepted", bad)
		}
	}
	d, _ := New(kernelSource(8, 1))
	encoded, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || !reflect.DeepEqual(decoded, d) {
		t.Fatalf("valid document roundtrip: %v", err)
	}
	for _, raw := range [][]byte{
		append([]byte(`{"future":true,`), encoded[1:]...),
		append([]byte(`{"SCHEMA":1,`), encoded[1:]...),
		bytes.Replace(encoded, []byte(`"source": {`), []byte(`"source": {"asset_id":"duplicate",`), 1),
		append(append([]byte(nil), encoded...), []byte(` {}`)...),
		append(append([]byte(nil), encoded...), 0xff),
		bytes.Repeat([]byte(" "), maxDocumentJSONBytes+1),
	} {
		if _, err := Decode(raw); err == nil {
			t.Error("unknown, trailing, invalid UTF-8 or oversized document accepted")
		}
	}
	tooMany := d.Clone()
	tooMany.Operations = make([]Operation, maxDocumentOperations+1)
	if err := tooMany.Validate(); err == nil {
		t.Fatal("too many operations accepted")
	}
	tooMany = d.Clone()
	tooMany.Markers = make([]Marker, maxDocumentMarkers+1)
	if err := tooMany.Validate(); err == nil {
		t.Fatal("too many markers accepted")
	}
	tooMuch := d.Clone()
	tooMuch.Markers = []Marker{{ID: "m", Label: strings.Repeat("x", maxDocumentTextBytes), Frame: 1}}
	if err := tooMuch.Validate(); err == nil {
		t.Fatal("oversized labels accepted")
	}
	badText := d.Clone()
	badText.Markers = []Marker{{ID: "m", Label: string([]byte{0xff}), Frame: 1}}
	if err := badText.Validate(); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestKernelBoundedHistoryStaleSaveAndRevisionExhaustion(t *testing.T) {
	d, _ := New(kernelSource(8, 1))
	if _, err := Open(d, maxHistoryLimit+1); err == nil {
		t.Fatal("oversized history accepted")
	}
	s, err := Open(d, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := s.Mutate(uint64(i), func(next *Document) error { next.BPM++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkSaved(1); err == nil || !s.Dirty() {
		t.Fatal("stale save cleared dirty state")
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().BPM != 1 || s.Undo() == nil {
		t.Fatal("bounded undo history failed")
	}
	if _, err := s.Mutate(s.Snapshot().Revision, func(next *Document) error { next.BPM = 42; return nil }); err != nil || s.CanRedo() {
		t.Fatal("new mutation failed to clear redo", err)
	}
	if _, err := s.Mutate(s.Snapshot().Revision, nil); err == nil {
		t.Fatal("nil mutation accepted")
	}
	d.Revision = ^uint64(0)
	exhausted, err := Open(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exhausted.Mutate(d.Revision, func(next *Document) error { next.BPM = 1; return nil }); err == nil || exhausted.Dirty() || exhausted.CanUndo() {
		t.Fatal("revision wrap mutated or dirtied the document")
	}
}

func TestKernelSnapshotsDuringUndoAndRedo(t *testing.T) {
	d, _ := New(kernelSource(8, 1))
	s, err := Open(d, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mutate(0, func(next *Document) error { AddMarker(next, Marker{ID: "m", Frame: 1}); return nil }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = s.Undo()
			_ = s.Redo()
		}
	}()
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				snapshot := s.Snapshot()
				if len(snapshot.Markers) > 0 {
					snapshot.Markers[0].Label = "private copy"
				}
				_ = s.Dirty()
				_ = s.CanUndo()
				_ = s.CanRedo()
			}
		}()
	}
	wg.Wait()
	if got := s.Snapshot(); len(got.Markers) != 1 || got.Markers[0].Label != "" {
		t.Fatalf("snapshot leaked into history: %+v", got)
	}
}

func TestKernelNormalizeMultiplierBounds(t *testing.T) {
	for _, pair := range [][2]float64{{math.NaN(), .5}, {math.Inf(1), .5}, {-1, .5}, {.5, 0}, {.5, 1.1}, {.5, math.NaN()}} {
		if _, err := NormalizeMultiplier(pair[0], pair[1]); err == nil {
			t.Fatalf("invalid peak or target accepted: %v", pair)
		}
	}
	for _, tc := range []struct{ peak, target, want float64 }{{0, 1, 1}, {.5, 1, 2}, {math.SmallestNonzeroFloat64, 1, 100}} {
		got, err := NormalizeMultiplier(tc.peak, tc.target)
		if err != nil || got != tc.want {
			t.Fatalf("peak %g: got %g, want %g: %v", tc.peak, got, tc.want, err)
		}
	}
}

func TestKernelRenderBudgetExactBoundary(t *testing.T) {
	for _, channels := range []int{1, 2, 32} {
		limit := maxRenderBytes / int64(16+8*channels)
		if err := validateWindowBudget(Range{0, limit}, channels, int(limit)); err != nil {
			t.Fatalf("%d channels: exact budget rejected: %v", channels, err)
		}
		if err := validateWindowBudget(Range{0, limit + 1}, channels, int(limit+1)); err == nil {
			t.Fatalf("%d channels: over-budget render accepted", channels)
		}
	}
}

type boundedSyntheticReader struct {
	maxRequest int
	calls      int
}

func (r *boundedSyntheticReader) ReadFrames(ctx context.Context, _ int64, n int) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n <= 0 || n > 8192 {
		return nil, errors.New("synthetic reader request exceeds 8192 frames")
	}
	r.calls++
	r.maxRequest = max(r.maxRequest, n)
	pcm := make([]float32, n*2)
	for frame := 0; frame < n; frame++ {
		pcm[frame*2], pcm[frame*2+1] = .5, -.25
	}
	return pcm, nil
}

// This synthesizes frames on demand; it neither opens an audio file nor plays
// audio and is not an end-to-end codec, mixer, latency or GUI test.
func TestKernelSyntheticLongStereoBounded(t *testing.T) {
	const frames = 5_313_600 // 110.7 seconds at 48 kHz
	r := &boundedSyntheticReader{}
	p, err := BuildPyramid(context.Background(), r, frames, 2, 256)
	if err != nil {
		t.Fatal(err)
	}
	if _, first, last, err := p.Visible(4_800_000, 4_800_000+96_000, 900); err != nil || first >= last {
		t.Fatalf("100-second viewport: %d:%d, %v", first, last, err)
	}
	d, _ := New(kernelSource(frames, 2))
	d.Operations = []Operation{{Kind: Gain, Region: Range{0, frames}, Value: 2}}
	peak, err := MeasurePeak(context.Background(), r, d, Range{0, frames})
	if err != nil || peak != 1 {
		t.Fatalf("effective peak %g: %v", peak, err)
	}
	multiplier, err := NormalizeMultiplier(peak, .5)
	if err != nil {
		t.Fatal(err)
	}
	d.Operations = append(d.Operations, Operation{Kind: Normalize, Region: Range{0, frames}, Value: multiplier})
	window, err := RenderWindow(context.Background(), r, d, Range{4_800_000, 4_802_048}, 2048)
	if err != nil || len(window) != 4096 || window[0] != .5 || window[1] != -.25 {
		t.Fatalf("100-second rendered window %v: %v", window[:min(len(window), 2)], err)
	}
	if r.maxRequest > 8192 || r.calls < 2 {
		t.Fatalf("unbounded reader requests: max %d, calls %d", r.maxRequest, r.calls)
	}
}
