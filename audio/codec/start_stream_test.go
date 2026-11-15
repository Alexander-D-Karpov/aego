package codec

import (
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"aego/audio/mix"
)

func startDecoder(frames int64) *observedSeekDecoder {
	return &observedSeekDecoder{info: Info{SampleRate: 48000, Channels: 1, Frames: frames},
		infos: make(chan struct{}, 16), seeks: make(chan int64, 16), gate: make(chan struct{})}
}

func streamResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish")
		return nil
	}
}

func TestStreamStartFrameAndLoopPCM(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start int64
		loop  bool
	}{
		{"ordinary", 400, false},
		{"loop-wrap", 445, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := startDecoder(1000)
			s := mix.NewStream(48000, 1, 64)
			cancel := make(chan struct{})
			done := Stream(d, s, StreamOptions{StartFrame: tc.start, Loop: tc.loop,
				LoopStart: 300, LoopEnd: 450, Chunk: 16}, cancel)
			t.Cleanup(func() {
				close(cancel)
				if err := streamResult(t, done); err != nil {
					t.Errorf("stream: %v", err)
				}
			})
			waitStreamRing(t, s, 64)
			pcm := make([]float32, 64)
			if n := s.Ring.Read(pcm); n != len(pcm) {
				t.Fatalf("read %d frames", n)
			}
			for i, got := range pcm {
				want := tc.start + int64(i)
				if tc.loop && want >= 450 {
					want = 300 + (want-300)%150
				}
				if got != float32(want) {
					t.Fatalf("frame %d = %v, want %d", i, got, want)
				}
			}
		})
	}
}

func TestStreamStartRejectsInvalidFrameAndFailedSeek(t *testing.T) {
	fault := errors.New("initial seek failed")
	for _, tc := range []struct {
		name string
		opts StreamOptions
		err  error
	}{
		{"negative", StreamOptions{StartFrame: -1}, nil},
		{"end", StreamOptions{StartFrame: 1000}, nil},
		{"past-loop", StreamOptions{StartFrame: 450, Loop: true, LoopStart: 300, LoopEnd: 450}, nil},
		{"decoder-error", StreamOptions{StartFrame: 400}, fault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := startDecoder(1000)
			d.seekErr = tc.err
			s := mix.NewStream(48000, 1, 64)
			done := Stream(d, s, tc.opts, make(chan struct{}))
			err := streamResult(t, done)
			if err == nil || tc.err != nil && !errors.Is(err, tc.err) || !s.EOF() || s.Ring.Len() != 0 {
				t.Fatalf("invalid start result: err=%v eof=%v buffered=%d", err, s.EOF(), s.Ring.Len())
			}
		})
	}
	t.Run("cancelled before initial seek", func(t *testing.T) {
		d := startDecoder(1000)
		s := mix.NewStream(48000, 1, 64)
		cancel := make(chan struct{})
		close(cancel)
		if err := streamResult(t, Stream(d, s, StreamOptions{StartFrame: 400}, cancel)); err != nil {
			t.Fatal(err)
		}
		if len(d.seeks) != 0 || s.Ring.Len() != 0 {
			t.Fatal("cancelled stream sought or published PCM")
		}
	})
}

func TestStreamStartCombinesSkipDuringInitialSeek(t *testing.T) {
	d := startDecoder(1000)
	d.block.Store(true)
	s := mix.NewStream(48000, 1, 64)
	cancel := make(chan struct{})
	done := Stream(d, s, StreamOptions{StartFrame: 400, Chunk: 16}, cancel)
	var once sync.Once
	openGate := func() { once.Do(func() { close(d.gate) }) }
	t.Cleanup(func() {
		openGate()
		close(cancel)
		if err := streamResult(t, done); err != nil {
			t.Errorf("stream: %v", err)
		}
	})
	waitDecoderInfo(t, d)
	waitDecoderInfo(t, d)
	select {
	case got := <-d.seeks:
		if got != 400 {
			t.Fatalf("initial seek = %d", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("initial seek did not begin")
	}
	m, err := mix.New(mix.Config{SampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Play(mix.PlayParams{Stream: s, Gain: 1}).Valid() {
		t.Fatal("could not start stream")
	}
	m.Skip(32)
	openGate()
	waitDecoderInfo(t, d) // The feeder accounted for the skip while held.
	select {
	case got := <-d.seeks:
		t.Fatalf("seek to %d before the hold was released", got)
	default:
	}
	m.Mix(make([]float32, 2))
	select {
	case got := <-d.seeks:
		if got != 431 {
			t.Fatalf("combined seek = %d, want 431", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("combined seek did not begin")
	}
	waitStreamRing(t, s, 16)
	pcm := make([]float32, 16)
	if n := s.Ring.Read(pcm); n != len(pcm) {
		t.Fatalf("read %d frames", n)
	}
	for i, got := range pcm {
		if got != float32(431+i) {
			t.Fatalf("frame %d = %v, want %d", i, got, 431+i)
		}
	}
}

func TestStreamStartRejectsOverflowingSkip(t *testing.T) {
	d := startDecoder(0) // Unknown source length.
	d.block.Store(true)
	s := mix.NewStream(48000, 1, 64)
	cancel := make(chan struct{})
	done := Stream(d, s, StreamOptions{StartFrame: math.MaxInt64 - 3}, cancel)
	var once sync.Once
	openGate := func() { once.Do(func() { close(d.gate) }) }
	t.Cleanup(func() { openGate(); close(cancel) })
	waitDecoderInfo(t, d)
	waitDecoderInfo(t, d)
	select {
	case <-d.seeks:
	case <-time.After(5 * time.Second):
		t.Fatal("initial seek did not begin")
	}
	m, err := mix.New(mix.Config{SampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Play(mix.PlayParams{Stream: s, Gain: 1}).Valid() {
		t.Fatal("could not start stream")
	}
	m.Skip(10)
	openGate()
	err = streamResult(t, done)
	if err == nil || !strings.Contains(err.Error(), "overflows") || !s.EOF() || s.Ring.Len() != 0 {
		t.Fatalf("overflow result: err=%v eof=%v buffered=%d", err, s.EOF(), s.Ring.Len())
	}
}
