package codec

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aego/audio/mix"
)

type observedSeekDecoder struct {
	pos       int64
	info      Info
	infos     chan struct{}
	seeks     chan int64
	block     atomic.Bool
	gate      chan struct{}
	nextGate  chan struct{}
	seekCount int
	blockRead atomic.Bool
	reads     chan int64
	readGate  chan struct{}
	seekErr   error
}

func (d *observedSeekDecoder) Info() Info {
	d.infos <- struct{}{}
	return d.info
}

func (d *observedSeekDecoder) Read(dst []float32) (int, error) {
	if d.blockRead.Load() {
		d.reads <- d.pos
		<-d.readGate
	}
	n := min(len(dst), int(d.info.Frames-d.pos))
	if n <= 0 {
		return 0, io.EOF
	}
	for i := range n {
		dst[i] = float32(d.pos + int64(i))
	}
	d.pos += int64(n)
	return n, nil
}

func (d *observedSeekDecoder) SeekFrame(frame int64) error {
	d.seekCount++
	d.seeks <- frame
	if d.block.Load() {
		gate := d.gate
		if d.seekCount > 1 && d.nextGate != nil {
			gate = d.nextGate
		}
		<-gate
	}
	if d.seekErr != nil {
		return d.seekErr
	}
	d.pos = frame
	return nil
}

type heldStreamFixture struct {
	d          *observedSeekDecoder
	s          *mix.Stream
	m          *mix.Mixer
	id         mix.VoiceID
	cancel     chan struct{}
	cancelOnce sync.Once
	done       <-chan error
	consumed   bool
}

func newHeldStreamFixture(t *testing.T, seekErr error) *heldStreamFixture {
	t.Helper()
	const rate = 48000
	d := &observedSeekDecoder{info: Info{SampleRate: rate, Channels: 1, Frames: 1000}, infos: make(chan struct{}, 16), seeks: make(chan int64, 16), gate: make(chan struct{}), seekErr: seekErr}
	s := mix.NewStream(rate, 1, 64)
	f := &heldStreamFixture{d: d, s: s, cancel: make(chan struct{})}
	f.done = Stream(d, s, StreamOptions{Chunk: 16}, f.cancel)
	t.Cleanup(func() {
		f.stop()
		if !f.consumed {
			select {
			case err := <-f.done:
				if err != nil {
					t.Errorf("stream feeder: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("stream feeder did not cancel")
			}
		}
	})
	waitDecoderInfo(t, d)
	waitStreamRing(t, s, s.Ring.Cap())
	var err error
	f.m, err = mix.New(mix.Config{SampleRate: rate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	f.id = f.m.Play(mix.PlayParams{Stream: s, Gain: 1})
	if !f.id.Valid() {
		t.Fatal("could not start stream voice")
	}
	f.m.Skip(0)
	return f
}

func (f *heldStreamFixture) stop() {
	f.cancelOnce.Do(func() { close(f.cancel) })
}

func (f *heldStreamFixture) result(t *testing.T) error {
	t.Helper()
	select {
	case err := <-f.done:
		f.consumed = true
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("stream feeder did not finish")
		return nil
	}
}

func TestHeldStreamNonLoopEOFAndCancel(t *testing.T) {
	t.Run("end while held", func(t *testing.T) {
		f := newHeldStreamFixture(t, nil)
		f.m.Skip(1000) // The interpolation cache still needs frame 999.
		waitDecoderInfo(t, f.d)
		if f.s.EOF() {
			t.Fatal("stream ended before its last frame")
		}
		f.m.Skip(1)
		if err := f.result(t); err != nil {
			t.Fatal(err)
		}
		if !f.s.EOF() {
			t.Fatal("stream did not publish EOF")
		}
		select {
		case frame := <-f.d.seeks:
			t.Fatalf("unnecessary seek to %d before EOF", frame)
		default:
		}
		f.m.Skip(1)
		ev, ok := f.m.Events().Pop()
		if !ok || ev.Kind != mix.EvFinished || ev.Voice != f.id {
			t.Fatalf("end event = %+v, %v; want finished voice %+v", ev, ok, f.id)
		}
	})
	t.Run("cancel while held", func(t *testing.T) {
		f := newHeldStreamFixture(t, nil)
		f.m.Skip(300)
		waitDecoderInfo(t, f.d)
		f.stop()
		if err := f.result(t); err != nil {
			t.Fatal(err)
		}
		if !f.s.EOF() {
			t.Fatal("cancel did not publish EOF")
		}
		select {
		case frame := <-f.d.seeks:
			t.Fatalf("seek to %d despite cancellation while held", frame)
		default:
		}
	})
}

func TestHeldStreamReportsDeferredSeekError(t *testing.T) {
	fault := errors.New("injected seek failure")
	f := newHeldStreamFixture(t, fault)
	f.m.Skip(300)
	waitDecoderInfo(t, f.d)
	select {
	case frame := <-f.d.seeks:
		t.Fatalf("seek to %d before release", frame)
	default:
	}
	f.m.Mix(make([]float32, 2))
	if err := f.result(t); !errors.Is(err, fault) {
		t.Fatalf("stream error = %v, want injected seek failure", err)
	}
	if !f.s.EOF() {
		t.Fatal("seek failure did not publish EOF")
	}
	select {
	case frame := <-f.d.seeks:
		if frame != 299 {
			t.Fatalf("seek = %d, want 299", frame)
		}
	default:
		t.Fatal("failed seek was not recorded")
	}
}

func (d *observedSeekDecoder) Close() error { return nil }

func waitStreamRing(t *testing.T, s *mix.Stream, frames int) {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for s.Ring.Len() < frames {
		select {
		case <-timeout.C:
			t.Fatalf("stream ring contains %d frames, want at least %d", s.Ring.Len(), frames)
		case <-tick.C:
		}
	}
}

func waitDecoderInfo(t *testing.T, d *observedSeekDecoder) {
	t.Helper()
	select {
	case <-d.infos:
	case <-time.After(5 * time.Second):
		t.Fatal("stream feeder did not process the skip")
	}
}

func TestHeldStreamDefersAndCombinesSeeks(t *testing.T) {
	const rate = 48000
	d := &observedSeekDecoder{info: Info{SampleRate: rate, Channels: 1, Frames: 1000}, infos: make(chan struct{}, 16), seeks: make(chan int64, 16), gate: make(chan struct{})}
	var gateOnce sync.Once
	openGate := func() { gateOnce.Do(func() { close(d.gate) }) }
	s := mix.NewStream(rate, 1, 64)
	cancel := make(chan struct{})
	done := Stream(d, s, StreamOptions{Loop: true, LoopStart: 100, LoopEnd: 1000, Chunk: 16}, cancel)
	t.Cleanup(func() {
		openGate()
		close(cancel)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stream feeder: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("stream feeder did not cancel")
		}
	})
	waitDecoderInfo(t, d) // Initial channel validation.
	waitStreamRing(t, s, s.Ring.Cap())
	m, err := mix.New(mix.Config{SampleRate: rate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	if id := m.Play(mix.PlayParams{Stream: s, Gain: 1}); !id.Valid() {
		t.Fatal("could not start stream voice")
	}
	m.Skip(0) // Process the play command without advancing the voice.
	m.Skip(3000)
	waitDecoderInfo(t, d)
	m.Skip(100)
	waitDecoderInfo(t, d)
	if !s.Held() {
		t.Fatal("stream should remain held while the mixer is skipping it")
	}
	select {
	case frame := <-d.seeks:
		t.Fatalf("decoder sought to %d while the stream was held", frame)
	default:
	}

	// Rendering releases the hold. All skips should resolve to one seek,
	// including wrapping around the 100..1000 loop.
	// Hold the fake decoder at SeekFrame until Mix returns, so the mixer cannot
	// consume any newly decoded samples before the test examines the ring.
	d.block.Store(true)
	m.Mix(make([]float32, 2))
	select {
	case frame := <-d.seeks:
		if frame != 399 {
			t.Fatalf("combined seek = %d, want 399", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not seek after release")
	}
	openGate()
	waitStreamRing(t, s, 8)
	samples := make([]float32, 8)
	if n := s.Ring.Read(samples); n != len(samples) {
		t.Fatalf("read %d decoded frames, want %d", n, len(samples))
	}
	for i, got := range samples {
		if want := float32(399 + i); got != want {
			t.Fatalf("sample %d = %v, want %v", i, got, want)
		}
	}
	select {
	case frame := <-d.seeks:
		t.Fatalf("unexpected additional seek to %d", frame)
	default:
	}
}

func TestStreamSkipDuringDeferredSeekDoesNotPublishOldFrames(t *testing.T) {
	const rate = 48000
	d := &observedSeekDecoder{info: Info{SampleRate: rate, Channels: 1, Frames: 1000}, infos: make(chan struct{}, 16), seeks: make(chan int64, 16), gate: make(chan struct{}), nextGate: make(chan struct{})}
	var gateOnce, secondOnce sync.Once
	openGate := func() { gateOnce.Do(func() { close(d.gate) }) }
	openSecond := func() { secondOnce.Do(func() { close(d.nextGate) }) }
	s := mix.NewStream(rate, 1, 64)
	cancel := make(chan struct{})
	done := Stream(d, s, StreamOptions{Chunk: 16}, cancel)
	t.Cleanup(func() {
		openGate()
		openSecond()
		close(cancel)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stream feeder: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("stream feeder did not cancel")
		}
	})
	waitDecoderInfo(t, d)
	waitStreamRing(t, s, s.Ring.Cap())
	m, err := mix.New(mix.Config{SampleRate: rate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	if id := m.Play(mix.PlayParams{Stream: s, Gain: 1}); !id.Valid() {
		t.Fatal("could not start stream voice")
	}
	m.Skip(0)
	m.Skip(300)
	waitDecoderInfo(t, d)

	// Resume and suspend inside the real feeder's SeekFrame. The mixer then
	// skips again before that seek completes, as it can when a device mutes.
	d.block.Store(true)
	m.Mix(make([]float32, 2))
	select {
	case frame := <-d.seeks:
		if frame != 299 {
			t.Fatalf("first seek = %d, want 299", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deferred seek did not start")
	}
	m.Skip(100)
	openGate()
	waitDecoderInfo(t, d)
	if got := s.Ring.Len(); got != 0 {
		t.Fatalf("published %d frames from the old seek after the stream was held again", got)
	}
	m.Mix(make([]float32, 2))
	select {
	case frame := <-d.seeks:
		if frame != 399 {
			t.Fatalf("second seek = %d, want 399", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second deferred seek did not start")
	}
	openSecond()
	waitStreamRing(t, s, 8)
	samples := make([]float32, 8)
	if n := s.Ring.Read(samples); n != len(samples) {
		t.Fatalf("read %d frames, want %d", n, len(samples))
	}
	for i, got := range samples {
		if want := float32(399 + i); got != want {
			t.Fatalf("sample %d = %v, want %v", i, got, want)
		}
	}
}

func TestStreamSkipDuringReadDoesNotPublishOldFrames(t *testing.T) {
	const rate = 48000
	d := &observedSeekDecoder{
		info:  Info{SampleRate: rate, Channels: 1, Frames: 1000},
		infos: make(chan struct{}, 16), seeks: make(chan int64, 16),
		reads: make(chan int64, 16), readGate: make(chan struct{}), nextGate: make(chan struct{}),
	}
	var readOnce, secondOnce sync.Once
	openRead := func() { readOnce.Do(func() { close(d.readGate) }) }
	openSecond := func() { secondOnce.Do(func() { close(d.nextGate) }) }
	s := mix.NewStream(rate, 1, 64)
	cancel := make(chan struct{})
	done := Stream(d, s, StreamOptions{Chunk: 16}, cancel)
	t.Cleanup(func() {
		openRead()
		openSecond()
		close(cancel)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stream feeder: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("stream feeder did not cancel")
		}
	})
	waitDecoderInfo(t, d)
	waitStreamRing(t, s, s.Ring.Cap())
	m, err := mix.New(mix.Config{SampleRate: rate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	if id := m.Play(mix.PlayParams{Stream: s, Gain: 1}); !id.Valid() {
		t.Fatal("could not start stream voice")
	}
	m.Skip(0)
	m.Skip(300)
	waitDecoderInfo(t, d)
	d.blockRead.Store(true)
	m.Mix(make([]float32, 2))
	select {
	case frame := <-d.reads:
		if frame != 299 {
			t.Fatalf("decoder read at %d, want 299", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("decoder read did not start")
	}
	m.Skip(100)
	openRead()
	waitDecoderInfo(t, d)
	if got := s.Ring.Len(); got != 0 {
		t.Fatalf("published %d stale frames after a skip during decoding", got)
	}
	// The first seek happened before the blocked Read; hold the second so the
	// mixer cannot consume its output before the test examines it.
	d.block.Store(true)
	m.Mix(make([]float32, 2))
	select {
	case frame := <-d.seeks:
		if frame != 299 {
			t.Fatalf("first seek = %d, want 299", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first seek was not recorded")
	}
	select {
	case frame := <-d.seeks:
		if frame != 399 {
			t.Fatalf("second seek = %d, want 399", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second deferred seek did not start")
	}
	openSecond()
	waitStreamRing(t, s, 8)
	samples := make([]float32, 8)
	if n := s.Ring.Read(samples); n != len(samples) {
		t.Fatalf("read %d frames, want %d", n, len(samples))
	}
	for i, got := range samples {
		if want := float32(399 + i); got != want {
			t.Fatalf("sample %d = %v, want %v", i, got, want)
		}
	}
}
