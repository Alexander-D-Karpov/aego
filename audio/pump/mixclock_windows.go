package pump

import (
	"math/bits"
	"syscall"
	"time"
	"unsafe"
)

var perfClock = newPerformanceClock()

type performanceClock struct {
	counter   uintptr
	frequency int64
}

// Go's Windows monotonic clock uses interrupt time, whose resolution can be
// coarser than a mix block. QPC measures those blocks without inventing a
// minimum duration. Keep a Go clock reading as a fallback if QPC fails.
type mixTimer struct {
	start time.Time
	ticks int64
	qpc   bool
}

func newPerformanceClock() performanceClock {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	counter := kernel32.NewProc("QueryPerformanceCounter")
	frequencyProc := kernel32.NewProc("QueryPerformanceFrequency")
	if counter.Find() != nil || frequencyProc.Find() != nil {
		return performanceClock{}
	}
	var frequency int64
	ok, _, _ := syscall.SyscallN(frequencyProc.Addr(), uintptr(unsafe.Pointer(&frequency)))
	if ok == 0 || frequency <= 0 {
		return performanceClock{}
	}
	return performanceClock{counter: counter.Addr(), frequency: frequency}
}

func performanceTicks() (int64, bool) {
	var ticks int64
	// SyscallN keeps the pointer alive for the call without forcing a heap
	// allocation for each block, unlike LazyProc.Call.
	ok, _, _ := syscall.SyscallN(perfClock.counter, uintptr(unsafe.Pointer(&ticks)))
	return ticks, ok != 0 && ticks >= 0
}

func startMixTimer() mixTimer {
	t := mixTimer{start: time.Now()}
	if perfClock.frequency > 0 {
		t.ticks, t.qpc = performanceTicks()
	}
	return t
}

func (t mixTimer) elapsed() time.Duration {
	if t.qpc {
		if end, ok := performanceTicks(); ok && end >= t.ticks {
			return counterDuration(end-t.ticks, perfClock.frequency)
		}
	}
	return time.Since(t.start)
}

// counterDuration converts a difference of counter readings, not an absolute
// uptime, to avoid overflow. Saturate if the elapsed time exceeds Duration.
func counterDuration(ticks, frequency int64) time.Duration {
	if ticks <= 0 || frequency <= 0 {
		return 0
	}
	const maxDuration = time.Duration(1<<63 - 1)
	seconds := ticks / frequency
	if seconds > int64(maxDuration/time.Second) {
		return maxDuration
	}
	hi, lo := bits.Mul64(uint64(ticks%frequency), uint64(time.Second))
	nanoseconds, _ := bits.Div64(hi, lo, uint64(frequency))
	d := time.Duration(seconds) * time.Second
	if time.Duration(nanoseconds) > maxDuration-d {
		return maxDuration
	}
	return d + time.Duration(nanoseconds)
}
