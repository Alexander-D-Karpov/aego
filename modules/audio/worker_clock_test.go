package audio

import (
	"testing"
	"time"
)

func TestWorkerQuietClockAccumulatesFractionalFrames(t *testing.T) {
	start := time.Unix(1000, 0)
	var clock workerQuietClock
	if !clock.start(start, 44100) {
		t.Fatal("could not start clock")
	}
	var total int64
	for i := 1; i <= 1000; i++ {
		frames, ok := clock.advance(start.Add(time.Duration(i) * time.Microsecond))
		if !ok {
			t.Fatalf("advance %d failed", i)
		}
		total += frames
	}
	if total != 44 || clock.remainder != 100000000 {
		t.Fatalf("lost fractional frames: total=%d remainder=%d", total, clock.remainder)
	}
	frames, ok := clock.advance(start.Add(time.Second))
	if !ok || total+frames != 44100 || clock.remainder != 0 {
		t.Fatalf("one second advanced %d frames, remainder %d", total+frames, clock.remainder)
	}
}

func TestWorkerQuietClockRateChangeAndInvalidInput(t *testing.T) {
	start := time.Unix(1000, 0)
	var clock workerQuietClock
	if _, ok := clock.advance(start); ok || clock.setRate(48000) || clock.start(time.Time{}, 48000) ||
		clock.start(start, 0) || clock.start(start, maxWorkerSampleRate+1) {
		t.Fatal("accepted an invalid clock state or rate")
	}
	if !clock.start(start, 48000) {
		t.Fatal("could not start clock")
	}
	first, ok := clock.advance(start.Add(500 * time.Millisecond))
	if !ok || first != 24000 || !clock.setRate(44100) {
		t.Fatal("could not advance at old rate")
	}
	second, ok := clock.advance(start.Add(time.Second))
	if !ok || second != 22050 {
		t.Fatalf("advanced %d frames at new rate", second)
	}
	before := clock
	if _, ok := clock.advance(start); ok || clock != before || clock.setRate(-1) || clock != before {
		t.Fatal("invalid transition changed the clock")
	}
}

func TestWorkerQuietClockRetainsFractionAcrossRateChange(t *testing.T) {
	start := time.Unix(1000, 0)
	var clock workerQuietClock
	clock.start(start, 48000)
	if n, ok := clock.advance(start.Add(10 * time.Microsecond)); !ok || n != 0 {
		t.Fatalf("unexpected first fragment: %d", n)
	}
	if !clock.setRate(52000) {
		t.Fatal("could not set new rate")
	}
	if n, ok := clock.advance(start.Add(20 * time.Microsecond)); !ok || n != 1 || clock.remainder != 0 {
		t.Fatalf("fraction did not survive rate transition: frames=%d remainder=%d", n, clock.remainder)
	}
}
