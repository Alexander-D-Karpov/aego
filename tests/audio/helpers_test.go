package audio

import (
	"math"
	"testing"

	"aego/audio/mix"
)

const rate = 48000

func tone(freq float64, frames int, amp float32) *mix.Memory {
	pcm := make([]float32, frames)
	for i := range pcm {
		pcm[i] = amp * float32(math.Sin(2*math.Pi*freq*float64(i)/rate))
	}
	return &mix.Memory{PCM: pcm, Rate: rate, Channels: 1}
}

func newMixer(t *testing.T) *mix.Mixer {
	t.Helper()
	m, err := mix.New(mix.Config{SampleRate: rate, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	return m
}
