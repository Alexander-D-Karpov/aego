package clock

import (
	"math"
	"testing"
)

func TestAccumulates(t *testing.T) {
	var s FixedStepper
	steps, alpha := s.Advance(1.0 / 120)
	if steps != 0 || math.Abs(alpha-0.5) > 1e-6 {
		t.Fatalf("first: steps=%d alpha=%f", steps, alpha)
	}
	steps, alpha = s.Advance(1.0 / 120)
	if steps != 1 || alpha > 1e-6 {
		t.Fatalf("second: steps=%d alpha=%f", steps, alpha)
	}
}

func TestNoSpiralAfterStall(t *testing.T) {
	var s FixedStepper
	steps, alpha := s.Advance(10)
	if steps != DefaultMaxSteps {
		t.Fatalf("steps=%d", steps)
	}
	if alpha < 0 || alpha >= 1 {
		t.Fatalf("alpha=%f", alpha)
	}
	steps, _ = s.Advance(0)
	if steps != 0 {
		t.Fatalf("backlog leaked into next frame: %d steps", steps)
	}
}

func TestNegativeAndNaN(t *testing.T) {
	var s FixedStepper
	if steps, _ := s.Advance(-1); steps != 0 {
		t.Fatal("negative delta produced steps")
	}
	if steps, _ := s.Advance(math.NaN()); steps != 0 {
		t.Fatal("NaN delta produced steps")
	}
}

func TestLongRunStepCount(t *testing.T) {
	s := FixedStepper{FixedDelta: 1.0 / 50}
	total := 0
	for i := 0; i < 600; i++ {
		steps, alpha := s.Advance(1.0 / 60)
		if alpha < 0 || alpha >= 1 {
			t.Fatalf("alpha out of range: %f", alpha)
		}
		total += steps
	}
	if total < 499 || total > 500 {
		t.Fatalf("total steps=%d", total)
	}
}
