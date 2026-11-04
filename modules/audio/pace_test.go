package audio

import (
	"testing"

	"aego/audio/mix"
	"aego/audio/pump"
	"aego/platform"
)

type refillSink struct{}

func (refillSink) Write(p []float32) int { return len(p) / 2 }
func (refillSink) Queued() int           { return 0 }
func (refillSink) Close()                {}

func TestFrameDrivenReserveRecoversDeviceLatency(t *testing.T) {
	mixer, err := mix.New(mix.Config{SampleRate: 48000, Channels: 2})
	if err != nil {
		t.Fatal(err)
	}
	m := Module{minBlocks: 4, spec: platform.AudioSpec{SampleRate: 48000, Channels: 2, Frames: 512}}
	m.pump = pump.New(mixer, refillSink{}, pump.Config{SampleRate: 48000, Channels: 2, Block: 512, Target: 4})
	fill := func(dt float64) int { m.pace(dt); return m.pump.Fill() }
	if n := fill(1.0 / 60); n != 4 {
		t.Fatalf("normal 60Hz: %d blocks", n)
	}
	if n := fill(.150); n != maxBlocks {
		t.Fatalf("did not react to a single 150ms frame: %d", n)
	}
	if n := fill(60); n != maxBlocks {
		t.Fatalf("unbounded after long pause: %d", n)
	}
	for range 45 {
		fill(1.0 / 60)
	}
	if n := fill(1.0 / 60); n != 4 {
		t.Fatalf("retained latency after recovery: %d", n)
	}
	if n := testing.AllocsPerRun(30, func() { fill(.095); fill(1.0 / 60) }); n != 0 {
		t.Fatalf("changing frame cadence allocated %.1f", n)
	}
}
