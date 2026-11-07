package pump

import (
	"syscall"
	"time"
	"unsafe"
)

// Go's Windows monotonic time can advance in coarse interrupt-time steps, so
// short audio blocks otherwise report zero despite doing work. QPC is the
// Windows monotonic high-resolution interval clock; it does not change global
// timer resolution or require a new dependency.
var (
	perfClock         = syscall.NewLazyDLL("kernel32.dll")
	perfCounterAddr   = perfClock.NewProc("QueryPerformanceCounter").Addr()
	perfFrequencyAddr = perfClock.NewProc("QueryPerformanceFrequency").Addr()
	perfClockRate     = readPerfFrequency()
)

func readPerfFrequency() uint64 {
	var value int64
	ok, _, _ := syscall.SyscallN(perfFrequencyAddr, uintptr(unsafe.Pointer(&value)))
	if ok == 0 || value <= 0 {
		return 0
	}
	return uint64(value)
}

func readPerfCounter() (int64, bool) {
	var value int64
	ok, _, _ := syscall.SyscallN(perfCounterAddr, uintptr(unsafe.Pointer(&value)))
	return value, ok != 0
}

type mixClockSample struct {
	counter  int64
	fallback time.Time
	highRes  bool
}

func mixClockStart() mixClockSample {
	if perfClockRate != 0 {
		if counter, ok := readPerfCounter(); ok {
			return mixClockSample{counter: counter, highRes: true}
		}
	}
	return mixClockSample{fallback: time.Now()}
}

func mixClockElapsed(s mixClockSample) time.Duration {
	if !s.highRes {
		return time.Since(s.fallback)
	}
	end, ok := readPerfCounter()
	if !ok || end <= s.counter {
		return 0
	}
	return counterDuration(uint64(end-s.counter), perfClockRate)
}
