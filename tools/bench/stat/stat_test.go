package stat

import (
	"math"
	"testing"
)

func seq(from, step float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = from + step*float64(i)
	}
	return out
}

func TestMannWhitney(t *testing.T) {
	// Disjoint samples of 10: U = 0, z = 49.5/sqrt(175), p = 0.000183.
	if p := MannWhitney(seq(1, 1, 10), seq(11, 1, 10)); math.Abs(p-0.000183) > 0.00001 {
		t.Fatalf("p = %v", p)
	}
	if p := MannWhitney(seq(1, 1, 10), seq(1, 1, 10)); p < 0.99 {
		t.Fatalf("identical samples p = %v", p)
	}
	if p := MannWhitney([]float64{5, 5, 5}, []float64{5, 5, 5}); p != 1 {
		t.Fatalf("all ties p = %v", p)
	}
}

func TestCompare(t *testing.T) {
	base := seq(100, 0.5, 10)
	cases := []struct {
		name string
		head []float64
		want bool
	}{
		{"same", seq(100, 0.5, 10), false},
		{"significant +30%", seq(130, 0.5, 10), true},
		{"significant +5%", seq(105, 0.5, 10), false},
		{"faster", seq(70, 0.5, 10), false},
		{"noise", []float64{60, 150, 90, 140, 80, 160, 70, 130, 100, 120}, false},
	}
	for _, c := range cases {
		if v := Compare(base, c.head, 0.05, 10); v.Regressed != c.want {
			t.Errorf("%s: %+v", c.name, v)
		}
	}
}
