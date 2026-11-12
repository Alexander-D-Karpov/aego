package audio

import (
	"math"
	"time"
)

// workerQuietClock advances only during an explicitly silent output interval.
// Its fractional frame is retained across updates and rate changes. The owner
// serializes calls and queues the returned frames at the same command boundary.
type workerQuietClock struct {
	at        time.Time
	rate      int64
	remainder int64 // billionths of one output frame
	started   bool
}

const maxWorkerSampleRate = 768000

func (c *workerQuietClock) start(now time.Time, rate int) bool {
	if now.IsZero() || rate <= 0 || rate > maxWorkerSampleRate {
		return false
	}
	*c = workerQuietClock{at: now, rate: int64(rate), started: true}
	return true
}

func (c *workerQuietClock) advance(now time.Time) (int64, bool) {
	if !c.started || now.IsZero() || now.Before(c.at) {
		return 0, false
	}
	elapsed := now.Sub(c.at)
	seconds := int64(elapsed / time.Second)
	nanos := int64(elapsed % time.Second)
	frac := nanos*c.rate + c.remainder // bounded by 1e9 * 768000
	extra := frac / int64(time.Second)
	if seconds > (math.MaxInt64-extra)/c.rate {
		return 0, false
	}
	frames := seconds*c.rate + extra
	c.at = now
	c.remainder = frac % int64(time.Second)
	return frames, true
}

// setRate must follow advance at the old rate at the exact transition time.
func (c *workerQuietClock) setRate(rate int) bool {
	if !c.started || rate <= 0 || rate > maxWorkerSampleRate {
		return false
	}
	c.rate = int64(rate)
	return true
}
