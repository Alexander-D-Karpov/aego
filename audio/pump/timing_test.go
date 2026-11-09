package pump

import (
	"testing"
	"time"

	"aego/audio/mix"
)

func TestRecordMixDurations(t *testing.T) {
	m, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	p := New(m, nil, Config{})
	// Zero is a valid observation when a block finishes within one clock tick.
	for _, d := range []time.Duration{0, 2 * time.Nanosecond, 5 * time.Nanosecond} {
		p.record(d)
	}
	s := p.Stats()
	if s.Blocks != 3 || s.MixMax != 5*time.Nanosecond || s.MixAvg != 2*time.Nanosecond {
		t.Fatalf("stats = %+v, want 3 blocks, max 5ns and average 2ns", s)
	}
}

func TestFillDoesNotAllocate(t *testing.T) {
	m, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	p := New(m, &sink{channels: 2}, Config{})
	p.Fill()
	if n := testing.AllocsPerRun(200, func() { p.Fill() }); n != 0 {
		t.Fatalf("steady-state Fill allocated %v times per call", n)
	}
}
