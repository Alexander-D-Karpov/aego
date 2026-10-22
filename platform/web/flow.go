package web

const ringFrames = 8192

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
