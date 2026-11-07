package pump

import (
	"math/bits"
	"time"
)

// counterDuration converts monotonic performance-counter ticks without losing
// sub-millisecond precision or overflowing on long intervals.
func counterDuration(ticks, frequency uint64) time.Duration {
	if frequency == 0 {
		return 0
	}
	const maxDuration = uint64(1<<63 - 1)
	seconds := ticks / frequency
	if seconds > maxDuration/uint64(time.Second) {
		return time.Duration(maxDuration)
	}
	hi, lo := bits.Mul64(ticks%frequency, uint64(time.Second))
	nanos, _ := bits.Div64(hi, lo, frequency)
	whole := seconds * uint64(time.Second)
	if nanos > maxDuration-whole {
		return time.Duration(maxDuration)
	}
	return time.Duration(whole + nanos)
}
