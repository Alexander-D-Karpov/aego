package pump

import (
	"math"
	"math/big"
	"math/rand"
	"testing"
	"time"
)

func TestCounterDuration(t *testing.T) {
	for _, tc := range []struct {
		ticks, frequency uint64
		want             time.Duration
	}{
		{0, 10_000_000, 0},
		{1, 0, 0},
		{1, 10_000_000, 100 * time.Nanosecond},
		{1, 3, 333_333_333 * time.Nanosecond},
		{15_000_000, 10_000_000, 1500 * time.Millisecond},
		{math.MaxUint64 - 1, math.MaxUint64, 999_999_999 * time.Nanosecond},
		{math.MaxUint64, 1, time.Duration(math.MaxInt64)},
	} {
		if got := counterDuration(tc.ticks, tc.frequency); got != tc.want {
			t.Errorf("counterDuration(%d, %d) = %v; want %v", tc.ticks, tc.frequency, got, tc.want)
		}
	}
}

func TestCounterDurationMatchesExactArithmetic(t *testing.T) {
	rng := rand.New(rand.NewSource(125))
	limit := big.NewInt(math.MaxInt64)
	for i := 0; i < 10000; i++ {
		ticks, frequency := rng.Uint64(), rng.Uint64()|1
		want := new(big.Int).SetUint64(ticks)
		want.Mul(want, big.NewInt(int64(time.Second)))
		want.Quo(want, new(big.Int).SetUint64(frequency))
		if want.Cmp(limit) > 0 {
			want.Set(limit)
		}
		if got := counterDuration(ticks, frequency); int64(got) != want.Int64() {
			t.Fatalf("ticks=%d frequency=%d: got %v, want %s ns", ticks, frequency, got, want)
		}
	}
}

func TestMixStatisticsRecordDurations(t *testing.T) {
	p := &Pump{}
	p.record(150 * time.Nanosecond)
	p.record(450 * time.Nanosecond)
	if p.st.Blocks != 2 || p.st.MixMax != 450*time.Nanosecond || p.st.MixAvg != 300*time.Nanosecond {
		t.Fatalf("incorrect measured mixing statistics: %+v", p.st)
	}
}
