package stat

import (
	"math"
	"sort"
)

type Verdict struct {
	Base      float64
	Head      float64
	Delta     float64
	P         float64
	Regressed bool
}

func Median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func MannWhitney(a, b []float64) float64 {
	n1, n2 := len(a), len(b)
	if n1 == 0 || n2 == 0 {
		return 1
	}
	type obs struct {
		v     float64
		first bool
	}
	all := make([]obs, 0, n1+n2)
	for _, v := range a {
		all = append(all, obs{v, true})
	}
	for _, v := range b {
		all = append(all, obs{v, false})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v < all[j].v })
	n := float64(n1 + n2)
	var r1, ties float64
	for i := 0; i < len(all); {
		j := i
		for j < len(all) && all[j].v == all[i].v {
			j++
		}
		rank := float64(i+j+1) / 2
		t := float64(j - i)
		ties += t*t*t - t
		for k := i; k < j; k++ {
			if all[k].first {
				r1 += rank
			}
		}
		i = j
	}
	u := r1 - float64(n1*(n1+1))/2
	mu := float64(n1*n2) / 2
	sigma := math.Sqrt(float64(n1*n2) / 12 * ((n + 1) - ties/(n*(n-1))))
	if sigma == 0 {
		return 1
	}
	z := max(0, (math.Abs(u-mu)-0.5)/sigma)
	return math.Erfc(z / math.Sqrt2)
}

func Compare(base, head []float64, alpha, tolerance float64) Verdict {
	v := Verdict{Base: Median(base), Head: Median(head), P: MannWhitney(base, head)}
	if v.Base > 0 {
		v.Delta = (v.Head - v.Base) / v.Base * 100
	}
	v.Regressed = v.P < alpha && v.Delta > tolerance
	return v
}
