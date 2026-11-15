package editor

import (
	"math"
	"strings"
	"testing"
)

func TestAudioRulerTicksFollowVisibleSourceTime(t *testing.T) {
	const rate = 48000
	ticks, n, step := audioRulerTicks(99*rate, 103*rate, rate, 320)
	if n != 5 || step != 1 {
		t.Fatalf("zoomed ruler: %d ticks at %v s", n, step)
	}
	for i := range n {
		if ticks[i].Seconds != float64(99+i) || ticks[i].X != float32(i*80) {
			t.Fatalf("tick %d: %+v", i, ticks[i])
		}
	}
	ticks, n, _ = audioRulerTicks(99*rate+1, 103*rate, rate, 320)
	if n != 4 || ticks[0].Seconds != 100 || ticks[0].X <= 0 {
		t.Fatalf("unaligned view started at wrong time: %+v", ticks[:n])
	}
}

func TestAudioRulerTicksAreBoundedAndAdaptToZoom(t *testing.T) {
	const rate = 48000
	for _, tc := range []struct {
		start, end int64
		width      float32
	}{
		{0, 5313600, 900},
		{100 * rate, 100*rate + 512, 900},
		{99 * rate, 103 * rate, 120},
		{0, 1, 1},
	} {
		ticks, n, step := audioRulerTicks(tc.start, tc.end, rate, tc.width)
		if n < 1 || n > audioRulerMaxTicks || step <= 0 {
			t.Fatalf("invalid tick count for %+v: %d %v", tc, n, step)
		}
		for i, tick := range ticks[:n] {
			if tick.X < 0 || tick.X > tc.width || i > 0 && tick.X <= ticks[i-1].X {
				t.Fatalf("invalid tick positions for %+v: %+v", tc, ticks[:n])
			}
			if i > 0 && tick.X-ticks[i-1].X < min(tc.width, audioRulerGap)-.01 {
				t.Fatalf("crowded tick positions for %+v: %+v", tc, ticks[:n])
			}
		}
	}
	if label := audioRulerLabel(100.002, .001); !strings.Contains(label, "100.002") {
		t.Fatalf("subsecond ruler lost precision: %q", label)
	}
}

func TestAudioRulerRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		start, end int64
		rate       int
		width      float32
	}{
		{-1, 1, 48000, 100}, {0, 0, 48000, 100}, {0, 1, 0, 100},
		{0, 1<<53 + 1, 48000, 100}, {0, 1, 48000, 0},
		{0, 1, 48000, float32(math.NaN())}, {0, 1, 48000, float32(math.Inf(1))},
	} {
		if _, n, _ := audioRulerTicks(tc.start, tc.end, tc.rate, tc.width); n != 0 {
			t.Fatalf("accepted invalid ruler %+v", tc)
		}
	}
}
