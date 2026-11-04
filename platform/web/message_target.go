package web

import "math"

const messageMinimumBlocks = 6

// messageTargetBlocks adds one block beyond a reported browser processing
// latency, subject to a six-block minimum and the existing twelve-block cap.
// Unlike the shared transport, posted messages are not bounded by a SAB ring.
func messageTargetBlocks(latency float64, rate, frames int) int {
	if rate <= 0 || frames <= 0 || latency <= 0 || math.IsNaN(latency) || math.IsInf(latency, 0) {
		return messageMinimumBlocks
	}
	blocks := latency * float64(rate) / float64(frames)
	if blocks >= 11 {
		return 12
	}
	return max(messageMinimumBlocks, int(math.Ceil(blocks))+1)
}
