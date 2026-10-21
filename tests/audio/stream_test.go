package audio

import (
	"bytes"
	"math"
	"testing"
	"time"

	"aego/audio/codec"
	"aego/audio/codec/wav"
	"aego/audio/mix"
)

func waitRing(t *testing.T, s *mix.Stream, frames int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.Ring.Len() < frames {
		if time.Now().After(deadline) {
			t.Fatalf("the decoder did not fill %d frames", frames)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestStreamLoopSeam(t *testing.T) {
	pcm := tone(480, 4800, 0.5).PCM
	dec, err := codec.Open(bytes.NewReader(wav.Encode(pcm, rate, 1, wav.EncodeOptions{Float: true})), "loop.wav")
	if err != nil {
		t.Fatal(err)
	}
	m := newMixer(t)
	st := mix.NewStream(rate, 1, 8192)
	cancel := make(chan struct{})
	done := codec.Stream(dec, st, codec.StreamOptions{Loop: true}, cancel)
	waitRing(t, st, 4096)
	m.Play(mix.PlayParams{Stream: st, Gain: 1})
	buf := make([]float32, 512*2)
	var prev float32
	worst := 0.0
	for i := 0; i < 40; i++ {
		waitRing(t, st, 600)
		m.Mix(buf)
		for f := 0; f < 512; f++ {
			worst = max(worst, math.Abs(float64(buf[2*f]-prev)))
			prev = buf[2*f]
		}
	}
	close(cancel)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if worst > 0.05 {
		t.Fatalf("a step of %.4f between samples, the loop seam clicks", worst)
	}
	for {
		ev, ok := m.Events().Pop()
		if !ok {
			break
		}
		if ev.Kind == mix.EvStarved {
			t.Fatal("the stream starved")
		}
	}
}
