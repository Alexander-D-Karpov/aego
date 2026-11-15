package codec_test

import (
	"math"
	"os"
	"testing"
	"time"

	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
)

// Decode sequentially from frame zero, independently of SeekFrame.
func sequentialFramesAt(t *testing.T, data []byte, hint string, start int64, count int) []float32 {
	t.Helper()
	d := open(t, data, hint)
	defer d.Close()
	ch := d.Info().Channels
	buf := make([]float32, 4096*ch)
	for pos := int64(0); pos < start; {
		frames := int(min(int64(4096), start-pos))
		n, err := d.Read(buf[:frames*ch])
		if err != nil || n <= 0 {
			t.Fatalf("sequential reference at %d: %d frames, %v", pos, n, err)
		}
		pos += int64(n)
	}
	out := make([]float32, count*ch)
	for pos := 0; pos < count; {
		n, err := d.Read(out[pos*ch:])
		if err != nil || n <= 0 {
			t.Fatalf("sequential reference at %d+%d: %d frames, %v", start, pos, n, err)
		}
		pos += n
	}
	return out
}

func checkRealStreamStart(t *testing.T, data []byte, hint string, start, loopStart int64, first, count int, tolerance float64) {
	t.Helper()
	d := open(t, data, hint)
	info := d.Info()
	if start+int64(first) > info.Frames || loopStart+int64(count-first) > info.Frames {
		t.Fatalf("fixture too short: %+v", info)
	}
	want := sequentialFramesAt(t, data, hint, start, first)
	want = append(want, sequentialFramesAt(t, data, hint, loopStart, count-first)...)
	s := mix.NewStream(info.SampleRate, info.Channels, count)
	cancel := make(chan struct{})
	done := codec.Stream(d, s, codec.StreamOptions{StartFrame: start, Loop: true,
		LoopStart: loopStart, LoopEnd: start + int64(first), Chunk: 64}, cancel)
	t.Cleanup(func() {
		close(cancel)
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("stream: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("stream did not cancel")
		}
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for s.Ring.Len() < count {
		select {
		case <-deadline.C:
			t.Fatalf("stream buffered %d frames, want %d", s.Ring.Len(), count)
		case <-tick.C:
		}
	}
	got := make([]float32, count*info.Channels)
	if n := s.Ring.Read(got); n != count {
		t.Fatalf("stream read %d frames, want %d", n, count)
	}
	for i, sample := range got {
		if diff := math.Abs(float64(sample - want[i])); diff > tolerance {
			t.Fatalf("sample %d = %v, sequential reference %v (difference %v)", i, sample, want[i], diff)
		}
	}
}

func TestStreamStartRealWAVAndLongVorbis(t *testing.T) {
	t.Run("synthetic stereo WAV", func(t *testing.T) {
		pcm := ramp(2048, 2)
		data := wav.Encode(pcm, 48000, 2, wav.EncodeOptions{Float: true})
		checkRealStreamStart(t, data, "synthetic.wav", 700, 200, 32, 128, 0)
	})
	t.Run("stereo Vorbis at 100 seconds", func(t *testing.T) {
		// Decode the tracked, licensed fixture in memory only; no audio device
		// or user-authored asset is opened for writing.
		data, err := os.ReadFile("../../examples/platformer/assets/audio/glorious-morning.ogg")
		if err != nil {
			t.Fatal(err)
		}
		checkRealStreamStart(t, data, "glorious-morning.ogg", 100*44100, 2*44100, 80, 256, 1e-3)
	})
}
