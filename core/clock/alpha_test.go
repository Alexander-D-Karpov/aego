package clock

import (
	"math/rand/v2"
	"testing"
)

func TestAlphaRange(t *testing.T) {
	var s FixedStepper
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 100000; i++ {
		if _, a := s.Advance(r.Float64() * 0.3); a < 0 || a >= 1 {
			t.Fatalf("alpha %v at step %d", a, i)
		}
	}
}
