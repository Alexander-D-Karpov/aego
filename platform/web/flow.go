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

type sendCount struct {
	sent     int64
	consumed int64
}

func (c *sendCount) queued() int {
	return max(0, int(c.sent-c.consumed))
}
