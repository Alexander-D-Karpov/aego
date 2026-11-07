package pump

import "testing"

func TestWindowsHighResolutionMixClock(t *testing.T) {
	if perfClockRate == 0 {
		t.Fatal("QueryPerformanceFrequency is unavailable")
	}
	start := mixClockStart()
	if !start.highRes {
		t.Fatal("QueryPerformanceCounter is unavailable")
	}
	var elapsed bool
	for i := 0; i < 10000; i++ {
		if mixClockElapsed(start) > 0 {
			elapsed = true
			break
		}
	}
	if !elapsed {
		t.Fatal("the high-resolution performance counter did not advance")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _ = mixClockElapsed(mixClockStart()) }); allocs != 0 {
		t.Fatalf("the audio timer allocated: %v allocations per measurement", allocs)
	}
}
