package web

import (
	"math"
	"testing"
)

func TestSharedTargetBlocks(t *testing.T) {
	tests := []struct {
		name    string
		latency float64
		rate    int
		frames  int
		want    int
	}{
		{"missing latency", 0, 48000, 512, 3},
		{"negative latency", -1, 48000, 512, 3},
		{"NaN latency", math.NaN(), 48000, 512, 3},
		{"infinite latency", math.Inf(1), 48000, 512, 3},
		{"invalid rate", 0.04, 0, 512, 3},
		{"invalid block", 0.04, 48000, 0, 3},
		{"small latency", 0.005, 48000, 512, 3},
		{"2048 frame burst", 2048.0 / 48000, 48000, 512, 5},
		{"fractional block", 0.04, 48000, 512, 5},
		{"44.1 kHz", 0.04, 44100, 512, 5},
		{"small blocks", 2048.0 / 48000, 48000, 256, 9},
		{"large blocks", 2048.0 / 48000, 48000, 2048, 3},
		{"pacing limit", 1, 48000, 256, 12},
		{"ring limit", 1, 48000, 2048, 4},
		{"block occupies ring", 1, 48000, ringFrames, 1},
		{"block exceeds ring", 1, 48000, ringFrames * 2, 1},
		{"overflow saturates", math.MaxFloat64, 48000, 512, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sharedTargetBlocks(tt.latency, tt.rate, tt.frames); got != tt.want {
				t.Fatalf("sharedTargetBlocks(%g, %d, %d) = %d, want %d", tt.latency, tt.rate, tt.frames, got, tt.want)
			}
		})
	}
}

func TestRingWrap(t *testing.T) {
	head := int32(math.MaxInt32 - 100)
	tail := head
	tail += 300
	if q := ringQueued(head, tail); q != 300 {
		t.Fatalf("queued across int32 wrap = %d", q)
	}
	if f := ringFree(head, tail, ringFrames); f != ringFrames-300 {
		t.Fatalf("free = %d", f)
	}
	pos, first := ringSpan(int32(ringFrames-10), ringFrames, 64)
	if pos != ringFrames-10 || first != 10 {
		t.Fatalf("span %d %d", pos, first)
	}
	p1, _ := ringSpan(math.MaxInt32, ringFrames, 1)
	var wrapped int32 = math.MaxInt32
	wrapped++
	p2, _ := ringSpan(wrapped, ringFrames, 1)
	if (p1+1)%ringFrames != p2 {
		t.Fatalf("positions %d and %d are not adjacent across the wrap", p1, p2)
	}
}

func TestSendCount(t *testing.T) {
	c := sendCount{sent: 2048, consumed: 512}
	if c.queued() != 1536 {
		t.Fatal(c.queued())
	}
	c.consumed = 4096
	if c.queued() != 0 {
		t.Fatal(c.queued())
	}
}

func TestSendCountClock(t *testing.T) {
	c := sendCount{sent: 4096, consumed: 2048}
	if q := c.queuedAt(5, 48000, 4096); q != 2048 {
		t.Fatalf("before any report = %d", q)
	}
	c.at = 1
	if q := c.queuedAt(1+1024.0/48000, 48000, 4096); q != 1024 {
		t.Fatalf("1024 frames after the report = %d", q)
	}
	if q := c.queuedAt(2, 48000, 4096); q != 0 {
		t.Fatalf("a second after the report = %d", q)
	}
	c.sent = 100000
	if q := c.queuedAt(60, 48000, 4096); q != 100000-2048-4096 {
		t.Fatalf("silent worklet = %d", q)
	}
}
