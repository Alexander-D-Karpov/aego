package audio

import (
	"math"
	"testing"

	"aego/core/mathx"
)

func TestSpatial(t *testing.T) {
	sp := Spatial{Min: 2, Max: 10, Width: 0.8, Rolloff: DefaultRolloff()}
	l := mathx.Vec2{}
	cases := []struct {
		at   mathx.Vec2
		gain float32
		pan  float32
	}{
		{mathx.Vec2{X: 0}, 1, 0},
		{mathx.Vec2{X: 1}, 1, 0.08},
		{mathx.Vec2{X: 6}, sp.rolloff(0.5), 0.48},
		{mathx.Vec2{X: 10}, 0, 0.8},
		{mathx.Vec2{X: -10}, 0, -0.8},
		{mathx.Vec2{X: -20}, 0, -0.8},
	}
	for _, c := range cases {
		g, p := sp.At(l, c.at)
		if math.Abs(float64(g-c.gain)) > 1e-5 || math.Abs(float64(p-c.pan)) > 1e-5 {
			t.Errorf("at %v: gain %v pan %v, want %v %v", c.at, g, p, c.gain, c.pan)
		}
	}
	if r := sp.rolloff(0.5); math.Abs(float64(r-0.25)) > 1e-5 {
		t.Fatalf("default rolloff at the middle is %v, want 0.25", r)
	}
	lin := Spatial{Min: 0, Max: 4}
	if g, _ := lin.At(l, mathx.Vec2{Y: 1}); math.Abs(float64(g-0.75)) > 1e-5 {
		t.Fatalf("an empty curve is linear: %v", g)
	}
}
