package codec_test

import (
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"aego/audio/codec"
	"aego/audio/mix"
)

// Observe a real decoder without changing its PCM or seek implementation.
type heldVorbisDecoder struct {
	codec.Decoder
	infos chan struct{}
	seeks chan int64
	gate  chan struct{}
}

func (d *heldVorbisDecoder) Info() codec.Info {
	d.infos <- struct{}{}
	return d.Decoder.Info()
}

func (d *heldVorbisDecoder) SeekFrame(frame int64) error {
	d.seeks <- frame
	<-d.gate
	return d.Decoder.SeekFrame(frame)
}

func TestHeldVorbisCombinedSeekAndLoopPCM(t *testing.T) {
	data, err := os.ReadFile("../../examples/platformer/assets/audio/glorious-morning.ogg")
	if err != nil {
		t.Fatal(err)
	}
	reference := open(t, data, "glorious-morning.ogg")
	defer reference.Close()
	info := reference.Info()
	if info.Channels != 2 || info.SampleRate != 44100 || info.Frames < 60000 {
		t.Fatalf("unexpected Vorbis fixture: %+v", info)
	}
	const loopStart, loopEnd = 50000, 60000
	pcm := make([]float32, loopEnd*info.Channels)
	for pos := 0; pos < loopEnd; {
		n, err := reference.Read(pcm[pos*info.Channels:])
		if err != nil || n == 0 {
			t.Fatalf("reference decode at %d: %d, %v", pos, n, err)
		}
		pos += n
	}

	decoder := &heldVorbisDecoder{
		Decoder: open(t, data, "glorious-morning.ogg"),
		infos:   make(chan struct{}, 16), seeks: make(chan int64, 16), gate: make(chan struct{}),
	}
	var gateOnce sync.Once
	openGate := func() { gateOnce.Do(func() { close(decoder.gate) }) }
	s := mix.NewStream(info.SampleRate, info.Channels, 256)
	cancel := make(chan struct{})
	done := codec.Stream(decoder, s, codec.StreamOptions{Loop: true, LoopStart: loopStart, LoopEnd: loopEnd, Chunk: 64}, cancel)
	t.Cleanup(func() {
		openGate()
		close(cancel)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Vorbis feeder: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Vorbis feeder did not cancel")
		}
		if err := decoder.Close(); err != nil {
			t.Error(err)
		}
	})
	waitInfo := func() {
		t.Helper()
		select {
		case <-decoder.infos:
		case <-time.After(5 * time.Second):
			t.Fatal("Vorbis feeder did not process the skip")
		}
	}
	waitRing := func(frames int) {
		t.Helper()
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for s.Ring.Len() < frames {
			select {
			case <-deadline.C:
				t.Fatalf("Vorbis ring has %d frames, want %d", s.Ring.Len(), frames)
			case <-ticker.C:
			}
		}
	}
	waitInfo()
	waitRing(s.Ring.Cap())
	m, err := mix.New(mix.Config{SampleRate: info.SampleRate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	if id := m.Play(mix.PlayParams{Stream: s, Gain: 1}); !id.Valid() {
		t.Fatal("could not start Vorbis stream")
	}
	m.Skip(0)
	m.Skip(129000)
	waitInfo()
	m.Skip(901)
	waitInfo()
	select {
	case frame := <-decoder.seeks:
		t.Fatalf("Vorbis sought to %d while held", frame)
	default:
	}
	// Block the seek until Mix returns, so it cannot consume the output in
	// parallel with this test. Mixing writes only to an in-memory buffer.
	m.Mix(make([]float32, 2))
	select {
	case frame := <-decoder.seeks:
		if frame != 59900 {
			t.Fatalf("combined Vorbis seek = %d, want 59900", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Vorbis seek did not start")
	}
	openGate()
	waitRing(256)
	got := make([]float32, 256*info.Channels)
	if n := s.Ring.Read(got); n != 256 {
		t.Fatalf("read %d frames, want 256", n)
	}
	for i, sample := range got {
		frame := 59900 + i/info.Channels
		if frame >= loopEnd {
			frame = loopStart + frame - loopEnd
		}
		want := pcm[frame*info.Channels+i%info.Channels]
		if math.Abs(float64(sample-want)) > 1e-3 {
			t.Fatalf("sample %d (source frame %d) = %g, want %g", i, frame, sample, want)
		}
	}
	select {
	case frame := <-decoder.seeks:
		if frame != loopStart {
			t.Fatalf("loop seek = %d, want %d", frame, loopStart)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Vorbis did not seek to the loop start")
	}
}

var _ codec.Decoder = (*heldVorbisDecoder)(nil)
