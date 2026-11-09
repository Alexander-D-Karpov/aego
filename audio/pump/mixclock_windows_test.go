package pump

import (
	"runtime"
	"testing"
	"time"
)

func TestCounterDuration(t *testing.T) {
	const maxInt64 = int64(1<<63 - 1)
	tests := []struct {
		name             string
		ticks, frequency int64
		want             time.Duration
	}{
		{"one tick", 1, 10_000_000, 100 * time.Nanosecond},
		{"seconds and fraction", 15_000_005, 10_000_000, time.Second + 500*time.Millisecond + 500*time.Nanosecond},
		{"large frequency without overflow", maxInt64 - 1, maxInt64, time.Second - time.Nanosecond},
		{"round down subnanosecond", 1, 10_000_000_000, 0},
		{"duration saturation", maxInt64, 1, time.Duration(maxInt64)},
		{"fractional saturation", 92_233_720_369, 10, time.Duration(maxInt64)},
		{"zero frequency", 1, 0, 0},
		{"negative ticks", -1, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := counterDuration(tt.ticks, tt.frequency); got != tt.want {
				t.Fatalf("counterDuration(%d, %d) = %v, want %v", tt.ticks, tt.frequency, got, tt.want)
			}
		})
	}
}

func TestMixTimerUsesPerformanceCounter(t *testing.T) {
	timer := startMixTimer()
	if perfClock.frequency <= 0 || !timer.qpc {
		t.Fatal("Windows performance counter unavailable")
	}
	deadline := time.After(time.Second)
	for timer.elapsed() <= 0 {
		select {
		case <-deadline:
			t.Fatal("Windows performance counter did not advance")
		default:
			runtime.Gosched()
		}
	}
}

func TestMixTimerDoesNotAllocate(t *testing.T) {
	if !startMixTimer().qpc {
		t.Fatal("Windows performance counter unavailable")
	}
	if n := testing.AllocsPerRun(1000, func() {
		timer := startMixTimer()
		_ = timer.elapsed()
	}); n != 0 {
		t.Fatalf("mix timer allocated %v times per block", n)
	}
}

func TestMixTimerFallsBackForInvalidCounter(t *testing.T) {
	timer := startMixTimer()
	if !timer.qpc {
		t.Fatal("Windows performance counter unavailable")
	}
	timer.ticks = 1<<63 - 1 // larger than any plausible current counter
	timer.start = time.Now().Add(-time.Second)
	if got := timer.elapsed(); got < time.Second {
		t.Fatalf("fallback duration = %v, want at least one second", got)
	}
}

func TestMixTimerFallsBackWithoutCounter(t *testing.T) {
	timer := mixTimer{start: time.Now().Add(-time.Second)}
	if got := timer.elapsed(); got < time.Second {
		t.Fatalf("fallback duration = %v, want at least one second", got)
	}
}
