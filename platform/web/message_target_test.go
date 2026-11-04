package web

import (
	"math"
	"testing"
)

func TestMessageTargetBlocks(t *testing.T) {
	for _, tc := range []struct {
		name         string
		latency      float64
		rate, frames int
		want         int
	}{
		{"zero latency", 0, 48000, 512, 6},
		{"negative latency", -1, 48000, 512, 6},
		{"NaN latency", math.NaN(), 48000, 512, 6},
		{"infinite latency", math.Inf(1), 48000, 512, 6},
		{"missing rate", 0.04, 0, 512, 6},
		{"missing frames", 0.04, 48000, 0, 6},
		{"2048 frame burst", 2048.0 / 48000, 48000, 512, 6},
		{"partial block", 2049.0 / 48000, 48000, 512, 6},
		{"five whole blocks", 2560.0 / 48000, 48000, 512, 6},
		{"more than five blocks", 2561.0 / 48000, 48000, 512, 7},
		{"bounded target", 1, 48000, 512, 12},
		{"overflow saturates", math.MaxFloat64, 48000, 512, 12},
		{"message transport is not ring bounded", 0.2, 48000, 2048, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageTargetBlocks(tc.latency, tc.rate, tc.frames); got != tc.want {
				t.Fatalf("target = %d, want %d", got, tc.want)
			}
		})
	}
}
