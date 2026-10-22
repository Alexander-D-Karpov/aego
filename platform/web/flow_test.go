package web

import (
	"math"
	"testing"
)

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
