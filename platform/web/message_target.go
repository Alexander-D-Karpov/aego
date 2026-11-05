package web

import "math"

const messageMinimumBlocks = 6

// messageTargetBlocks adds one block beyond reported browser processing latency, with a six-block minimum and twelve-block cap; posted messages have no SAB ring bound.
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
