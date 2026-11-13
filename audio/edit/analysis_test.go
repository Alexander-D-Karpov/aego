package audioedit

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestAnalyzeEditedStereoSelection(t *testing.T) {
	doc, err := New(kernelSource(6, 2))
	if err != nil {
		t.Fatal(err)
	}
	doc.Trim = Range{1, 5}
	doc.Operations = []Operation{
		{Kind: Reverse, Region: Range{0, 6}},
		{Kind: Gain, Region: Range{1, 5}, Value: 2},
	}
	samples := []float32{0, -.5, .25, -.25, .5, 0, .75, .25, 1, .5, -1, 1}
	before, source := doc.Clone(), append([]float32(nil), samples...)
	got, err := Analyze(context.Background(), fakePCM{channels: 2, samples: samples}, doc, Range{1, 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.Frames != 4 || len(got.Channels) != 2 {
		t.Fatalf("analysis dimensions: %+v", got)
	}
	want := []ChannelAnalysis{
		{Peak: 2, RMS: math.Sqrt(1.875), ClippedFrames: 3},
		{Peak: 1, RMS: math.Sqrt(.375), ClippedFrames: 1},
	}
	for i := range want {
		if got.Channels[i].Peak != want[i].Peak || math.Abs(got.Channels[i].RMS-want[i].RMS) > 1e-12 || got.Channels[i].ClippedFrames != want[i].ClippedFrames {
			t.Fatalf("channel %d: got %+v, want %+v", i, got.Channels[i], want[i])
		}
	}
	if !reflect.DeepEqual(doc, before) || !reflect.DeepEqual(samples, source) {
		t.Fatal("analysis changed its document or source PCM")
	}
}

type analysisReader struct {
	data   []float32
	calls  int
	max    int
	cancel context.CancelFunc
}

func (r *analysisReader) ReadFrames(_ context.Context, start int64, n int) ([]float32, error) {
	r.calls++
	r.max = max(r.max, n)
	if r.cancel != nil {
		r.cancel()
	}
	return append([]float32(nil), r.data[int(start):int(start)+n]...), nil
}

func TestAnalyzeBoundedChunksAndCancellation(t *testing.T) {
	const frames = 17001
	doc, _ := New(kernelSource(frames, 1))
	data := make([]float32, frames)
	data[8191], data[8192], data[frames-1] = -1, 2, -.5
	reader := &analysisReader{data: data}
	got, err := Analyze(context.Background(), reader, doc, Range{0, frames})
	if err != nil || reader.calls != 3 || reader.max != 8192 {
		t.Fatalf("analysis chunks: calls=%d max=%d err=%v", reader.calls, reader.max, err)
	}
	wantRMS := math.Sqrt((1 + 4 + .25) / float64(frames))
	if got.Frames != frames || len(got.Channels) != 1 || got.Channels[0].Peak != 2 ||
		math.Abs(got.Channels[0].RMS-wantRMS) > 1e-14 || got.Channels[0].ClippedFrames != 2 {
		t.Fatalf("analysis across chunks: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader = &analysisReader{data: data, cancel: cancel}
	got, err = Analyze(ctx, reader, doc, Range{0, frames})
	if !errors.Is(err, context.Canceled) || got.Frames != 0 || got.Channels != nil || reader.calls != 1 {
		t.Fatalf("canceled scan returned a partial result: %+v, %v, calls=%d", got, err, reader.calls)
	}
}

type analysisMutatingReader struct {
	doc  *Document
	data []float32
}

func (r *analysisMutatingReader) ReadFrames(_ context.Context, start int64, n int) ([]float32, error) {
	r.doc.Operations[0].Value = 0
	return append([]float32(nil), r.data[int(start):int(start)+n]...), nil
}

func TestAnalyzeKeepsAnImmutableOperationSnapshot(t *testing.T) {
	const frames = 9000
	doc, _ := New(kernelSource(frames, 1))
	doc.Operations = []Operation{{Kind: Gain, Region: Range{0, frames}, Value: 2}}
	data := make([]float32, frames)
	for i := range data {
		data[i] = .5
	}
	got, err := Analyze(context.Background(), &analysisMutatingReader{doc: &doc, data: data}, doc, Range{0, frames})
	if err != nil || len(got.Channels) != 1 || got.Channels[0] != (ChannelAnalysis{Peak: 1, RMS: 1, ClippedFrames: frames}) {
		t.Fatalf("reader changed the later analysis window: %+v, %v", got, err)
	}
}

func TestAnalyzeMatchesWaveformClippingThreshold(t *testing.T) {
	samples := []float32{1, -1, 1.1, -1.1, .9, -.9, 0, 0}
	doc, _ := New(kernelSource(int64(len(samples)), 1))
	reader := fakePCM{channels: 1, samples: samples}
	got, err := Analyze(context.Background(), reader, doc, Range{0, int64(len(samples))})
	if err != nil || len(got.Channels) != 1 {
		t.Fatal("cannot analyze full-scale samples", err)
	}
	pyramid, err := BuildPyramid(context.Background(), reader, int64(len(samples)), 1, 32)
	if err != nil || len(pyramid.Levels) == 0 || len(pyramid.Levels[0].Samples) != 1 {
		t.Fatal("cannot build comparison waveform", err)
	}
	if got.Channels[0].ClippedFrames != 4 || got.Channels[0].ClippedFrames != int64(pyramid.Levels[0].Samples[0].Clipped) {
		t.Fatalf("clipping threshold differs from waveform: analysis=%+v, waveform=%+v", got.Channels[0], pyramid.Levels[0].Samples[0])
	}
}

func TestAnalyzeFiniteAmplificationAndSilence(t *testing.T) {
	doc, _ := New(kernelSource(2, 1))
	for range 60 {
		doc.Operations = append(doc.Operations, Operation{Kind: Gain, Region: Range{0, 2}, Value: 100})
	}
	sample := float32(math.MaxFloat32)
	got, err := Analyze(context.Background(), fakePCM{channels: 1, samples: []float32{sample, -sample}}, doc, Range{0, 2})
	if err != nil || len(got.Channels) != 1 {
		t.Fatal("finite amplified signal failed", err)
	}
	a := got.Channels[0]
	if math.IsInf(a.RMS, 0) || math.IsNaN(a.RMS) || a.Peak <= 0 || a.RMS != a.Peak || a.ClippedFrames != 2 {
		t.Fatalf("amplified analysis overflowed: %+v", a)
	}
	silent, err := Analyze(context.Background(), fakePCM{channels: 1, samples: []float32{0, 0}}, doc, Range{0, 2})
	if err != nil || len(silent.Channels) != 1 || silent.Channels[0] != (ChannelAnalysis{}) {
		t.Fatalf("silent signal: %+v, %v", silent, err)
	}
}

func TestAnalyzeRejectsInvalidAndNonFiniteInputs(t *testing.T) {
	doc, _ := New(kernelSource(2, 1))
	valid := fakePCM{channels: 1, samples: []float32{0, 0}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		reader Reader
		range_ Range
	}{
		{"nil context", nil, valid, Range{0, 2}},
		{"nil reader", context.Background(), nil, Range{0, 2}},
		{"canceled", ctx, valid, Range{0, 2}},
		{"empty range", context.Background(), valid, Range{1, 1}},
		{"outside trim", context.Background(), valid, Range{0, 3}},
		{"short PCM", context.Background(), shortKernelReader{}, Range{0, 2}},
		{"NaN PCM", context.Background(), fakePCM{channels: 1, samples: []float32{0, float32(math.NaN())}}, Range{0, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Analyze(tc.ctx, tc.reader, doc, tc.range_)
			if err == nil || got.Frames != 0 || got.Channels != nil {
				t.Fatalf("invalid input returned %+v, %v", got, err)
			}
		})
	}
}
