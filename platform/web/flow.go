package web

import "math"

const ringFrames = 8192

// sharedTargetBlocks estimates one block beyond reported base latency, bounded by the ring and twelve-block pacing limit; browser scheduling is not guaranteed.
func sharedTargetBlocks(latency float64, rate, frames int) int {
	if frames <= 0 {
		return 3
	}
	limit := min(12, max(1, ringFrames/frames))
	minimum := min(3, limit)
	if rate <= 0 || latency <= 0 || math.IsNaN(latency) || math.IsInf(latency, 0) {
		return minimum
	}
	blocks := latency * float64(rate) / float64(frames)
	if blocks >= float64(limit-1) {
		return limit
	}
	return max(minimum, int(math.Ceil(blocks))+1)
}

func ringQueued(head, tail int32) int {
	return int(tail - head)
}

func ringFree(head, tail int32, size int) int {
	return size - ringQueued(head, tail)
}

func ringSpan(tail int32, size, n int) (pos, first int) {
	pos = int(uint32(tail) & uint32(size-1))
	return pos, min(n, size-pos)
}

// sendCount tracks the frames posted to the worklet against its last report of what it played.
type sendCount struct {
	sent     int64
	consumed int64
	at       float64
}

func (c *sendCount) queued() int {
	return max(0, int(c.sent-c.consumed))
}

// queuedAt also counts what the worklet played since its last report, by the audio clock, up to limit frames.
func (c *sendCount) queuedAt(now float64, rate, limit int) int {
	q := c.queued()
	if c.at <= 0 || now <= c.at {
		return q
	}
	played := min(int((now-c.at)*float64(rate)), limit)
	return max(0, q-played)
}
