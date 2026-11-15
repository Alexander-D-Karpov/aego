package audio

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aego/asset"
	"aego/audio/codec"
	"aego/audio/codec/wav"
)

func streamSeekFixture() (*asset.AudioArtifact, []float32) {
	pcm := make([]float32, 128*2)
	for i := range pcm {
		pcm[i] = float32(i-128) / 256
	}
	return &asset.AudioArtifact{Format: asset.AudioWAV, Rate: 48000, Channels: 2, Frames: 128, Stream: true,
		LoopStart: 10, LoopEnd: 60, Data: wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})}, pcm
}

func waitPreparedStream(t *testing.T, p *preparedStreamSeek, frames int) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for p.stream.Ring.Len() < frames {
		select {
		case <-deadline.C:
			t.Fatalf("prepared stream has %d frames, want %d", p.stream.Ring.Len(), frames)
		case <-tick.C:
		}
	}
}

func stopPreparedStream(t *testing.T, p *preparedStreamSeek) {
	t.Helper()
	p.cancel()
	p.cancel() // Ownership cleanup is safe to repeat.
	select {
	case <-p.done:
		if p.err != nil {
			t.Errorf("feeder failed: %v", p.err)
		}
	case <-time.After(5 * time.Second):
		t.Error("feeder did not stop")
	}
}

func TestPrepareStreamSeekPrimesPrecedingFrameAndLoop(t *testing.T) {
	a, pcm := streamSeekFixture()
	p, err := prepareStreamSeek(context.Background(), a, 58, true, 48000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopPreparedStream(t, p) })
	if p.origin != 57 || p.frame != 58 || p.initial != 1 || !p.loop || p.loopStart != 10 || p.loopEnd != 60 {
		t.Fatalf("invalid prepared coordinates: %+v", p)
	}
	waitPreparedStream(t, p, 8)
	got := make([]float32, 16)
	if n := p.stream.Ring.Read(got); n != 8 {
		t.Fatalf("read %d frames", n)
	}
	for i, sample := range got {
		frame := 57 + i/2
		if frame >= 60 {
			frame = 10 + frame - 60
		}
		want := pcm[frame*2+i%2]
		if sample != want {
			t.Fatalf("sample %d = %v, want %v", i, sample, want)
		}
	}
}

func TestPrepareStreamSeekSupportsTailAndRejectsInvalidRequests(t *testing.T) {
	a, pcm := streamSeekFixture()
	p, err := prepareStreamSeek(context.Background(), a, 127, false, 48000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopPreparedStream(t, p) })
	waitPreparedStream(t, p, 2)
	got := make([]float32, 4)
	if n := p.stream.Ring.Read(got); n != 2 || got[2] != pcm[254] || got[3] != pcm[255] {
		t.Fatalf("tail = %v (%d frames)", got, n)
	}
	for _, tc := range []struct {
		name  string
		a     *asset.AudioArtifact
		frame int64
		loop  bool
		rate  int
	}{
		{"negative", a, -1, false, 48000},
		{"end", a, 128, false, 48000},
		{"past-loop", a, 60, true, 48000},
		{"output-rate", a, 1, false, 8000},
		{"no-artifact", nil, 1, false, 48000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := prepareStreamSeek(context.Background(), tc.a, tc.frame, tc.loop, tc.rate); err == nil || got != nil {
				if got != nil {
					got.cancel()
				}
				t.Fatal("accepted invalid stream seek")
			}
		})
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := prepareStreamSeek(cancelled, a, 1, false, 48000); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation: %+v, %v", got, err)
	}
}

func TestPrepareStreamSeekLongVorbisMatchesSequentialPCM(t *testing.T) {
	// Decode the tracked fixture without opening an audio output or writing it.
	data, err := os.ReadFile("../../examples/platformer/assets/audio/glorious-morning.ogg")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := codec.Open(bytes.NewReader(data), ".ogg")
	if err != nil {
		t.Fatal(err)
	}
	defer reference.Close()
	info := reference.Info()
	target := int64(info.SampleRate) * 100
	if info.Channels != 2 || info.Frames <= target+64 {
		t.Fatalf("unexpected stereo fixture: %+v", info)
	}
	buf := make([]float32, 4096*info.Channels)
	for pos := int64(0); pos < target-1; {
		frames := int(min(int64(4096), target-1-pos))
		n, err := reference.Read(buf[:frames*info.Channels])
		if err != nil || n <= 0 {
			t.Fatalf("sequential decode at %d: %d, %v", pos, n, err)
		}
		pos += int64(n)
	}
	want := make([]float32, 64*info.Channels)
	for pos := 0; pos < 64; {
		n, err := reference.Read(want[pos*info.Channels:])
		if err != nil || n <= 0 {
			t.Fatalf("sequential window at %d: %d, %v", pos, n, err)
		}
		pos += n
	}
	a := &asset.AudioArtifact{Format: asset.AudioOgg, Rate: info.SampleRate, Channels: info.Channels,
		Frames: info.Frames, Stream: true, Data: data}
	p, err := prepareStreamSeek(context.Background(), a, target, false, 48000)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopPreparedStream(t, p) })
	waitPreparedStream(t, p, 64)
	got := make([]float32, len(want))
	if n := p.stream.Ring.Read(got); n != 64 {
		t.Fatalf("read %d frames", n)
	}
	for i, sample := range got {
		if diff := math.Abs(float64(sample - want[i])); diff > 1e-3 {
			t.Fatalf("sample %d = %v, sequential %v (difference %v)", i, sample, want[i], diff)
		}
	}
}

type blockedSeekDecoder struct {
	entered chan struct{}
	gate    chan struct{}
	closed  chan struct{}
	once    sync.Once
	reads   atomic.Int32
}

func (*blockedSeekDecoder) Info() codec.Info {
	return codec.Info{SampleRate: 48000, Channels: 2, Frames: 128}
}
func (d *blockedSeekDecoder) Read([]float32) (int, error) {
	d.reads.Add(1)
	return 0, nil
}
func (d *blockedSeekDecoder) SeekFrame(int64) error {
	close(d.entered)
	<-d.gate
	return nil
}
func (d *blockedSeekDecoder) Close() error {
	d.once.Do(func() { close(d.closed) })
	return nil
}

func TestPrepareStreamSeekCancellationDuringBlockedDecoder(t *testing.T) {
	a, _ := streamSeekFixture()
	d := &blockedSeekDecoder{entered: make(chan struct{}), gate: make(chan struct{}), closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	var gateOnce sync.Once
	release := func() { gateOnce.Do(func() { close(d.gate) }) }
	t.Cleanup(func() { cancel(); release() })
	type result struct {
		p   *preparedStreamSeek
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		p, err := prepareStreamSeekDecoder(ctx, a, 50, false, 48000, d)
		resultCh <- result{p, err}
	}()
	select {
	case <-d.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("decoder seek did not start")
	}
	cancel()
	select {
	case got := <-resultCh:
		t.Fatalf("worker returned while its decoder was still blocked: %+v", got)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	select {
	case got := <-resultCh:
		if got.p != nil || !errors.Is(got.err, context.Canceled) {
			t.Fatalf("cancel result = %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preparation did not return after decoder released")
	}
	select {
	case <-d.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("decoder did not close after the blocked call returned")
	}
	if d.reads.Load() != 0 {
		t.Fatal("cancelled preparation decoded stale PCM")
	}
}
