package pump

import (
	"testing"

	"aego/audio/mix"
)

// A browser can consume a posted block while the producer is still filling
// the remaining startup blocks. A priming output commits those blocks at once.
type primeSink struct {
	queued, staged, starts, ends, partial int
	priming                               bool
}

func (s *primeSink) BeginAudioPrime(frames int) {
	if s.priming || frames <= 0 {
		panic("invalid prime")
	}
	s.priming = true
	s.starts++
}

func (s *primeSink) EndAudioPrime() {
	if !s.priming {
		panic("prime was not started")
	}
	s.priming = false
	s.ends++
	s.queued += s.staged
	s.staged = 0
}

func (s *primeSink) Write(samples []float32) int {
	n := len(samples) / 2
	if s.priming {
		s.staged += n
	} else {
		s.partial++
		s.queued += n
	}
	return n
}

func (s *primeSink) Queued() int { return s.queued }
func (*primeSink) Close()        {}

func TestStartupFillPublishesOneCompletePrime(t *testing.T) {
	m, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	out := &primeSink{}
	p := New(m, out, Config{SampleRate: 48000, Channels: 2, Block: 512, Target: 5})
	if blocks := p.Fill(); blocks != 5 {
		t.Fatalf("first fill: %d blocks", blocks)
	}
	if out.starts != 1 || out.ends != 1 || out.partial != 0 || out.queued != 2560 {
		t.Fatalf("startup was not published as one complete prime: %+v", out)
	}
	// Once playing, blocks are published promptly; an actual subsequent empty
	// queue is not hidden behind another startup pre-roll.
	out.queued = 0
	if blocks := p.Fill(); blocks != 5 {
		t.Fatalf("subsequent fill: %d blocks", blocks)
	}
	if out.starts != 1 || out.ends != 1 || out.partial != 5 || out.queued != 2560 {
		t.Fatalf("steady-state playback was re-primed: %+v", out)
	}
}
