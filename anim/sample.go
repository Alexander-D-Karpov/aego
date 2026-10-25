package anim

import (
	"math"

	"aego/core/curve"
)

type Sampler struct {
	last []int32
}

func (s *Sampler) Reset(c *Clip) {
	n := len(c.Tracks)
	if cap(s.last) < n {
		s.last = make([]int32, n)
	}
	s.last = s.last[:n]
	clear(s.last)
}

func (s *Sampler) Sample(c *Clip, t float32, out []Value) {
	for i := range c.Tracks {
		out[i] = s.Track(c, i, t)
	}
}

func (s *Sampler) Track(c *Clip, i int, t float32) Value {
	tr := &c.Tracks[i]
	ks := tr.Keys
	n := len(ks)
	switch {
	case n == 0:
		return Value{}
	case t != t || t <= ks[0].T:
		return ks[0].V
	case t >= ks[n-1].T:
		return ks[n-1].V
	}
	lo := s.find(i, ks, t)
	return segment(tr, &ks[lo], &ks[lo+1], t)
}

func (s *Sampler) find(i int, ks []Key, t float32) int {
	n := len(ks)
	if i < len(s.last) {
		j := int(s.last[i])
		if j < n-1 && ks[j].T <= t && t < ks[j+1].T {
			return j
		}
		if j+2 < n && ks[j+1].T <= t && t < ks[j+2].T {
			s.last[i] = int32(j + 1)
			return j + 1
		}
	}
	lo, hi := 0, n-1
	for hi-lo > 1 {
		m := (lo + hi) / 2
		if ks[m].T <= t {
			lo = m
		} else {
			hi = m
		}
	}
	if i < len(s.last) {
		s.last[i] = int32(lo)
	}
	return lo
}

func segment(tr *Track, a, b *Key, t float32) Value {
	d := b.T - a.T
	if d <= 0 {
		return b.V
	}
	if a.Interp == curve.InterpStep {
		return a.V
	}
	u := (t - a.T) / d
	switch classOf(tr.Kind) {
	case classLanes:
		return lanes(tr, a, b, u, d)
	case classQuat:
		return Value{L: slerp(a.V.L, b.V.L, u)}
	case classInt:
		if tr.Numeric {
			return Int(int64(math.Round(float64(a.V.I) + float64(b.V.I-a.V.I)*float64(u))))
		}
	}
	return a.V
}

func lanes(tr *Track, a, b *Key, u, d float32) Value {
	var v Value
	n := lanesOf(tr.Kind)
	if a.Interp == curve.InterpLinear {
		for l := 0; l < n; l++ {
			v.L[l] = a.V.L[l] + (b.V.L[l]-a.V.L[l])*u
		}
		return v
	}
	u2 := u * u
	u3 := u2 * u
	h00 := 2*u3 - 3*u2 + 1
	h10 := u3 - 2*u2 + u
	h01 := 3*u2 - 2*u3
	h11 := u3 - u2
	for l := 0; l < n; l++ {
		v.L[l] = h00*a.V.L[l] + h10*d*a.Out[l] + h01*b.V.L[l] + h11*d*b.In[l]
	}
	return v
}

func Slerp(a, b [4]float32, t float32) [4]float32 {
	return slerp(a, b, t)
}

func slerp(a, b [4]float32, t float32) [4]float32 {
	dot := float64(a[0]*b[0] + a[1]*b[1] + a[2]*b[2] + a[3]*b[3])
	if dot < 0 {
		for i := range b {
			b[i] = -b[i]
		}
		dot = -dot
	}
	wa, wb := 1-float64(t), float64(t)
	if dot < 0.9995 {
		th := math.Acos(dot)
		s := math.Sin(th)
		wa, wb = math.Sin(wa*th)/s, math.Sin(wb*th)/s
	}
	var o [4]float32
	var n float64
	for i := range o {
		x := wa*float64(a[i]) + wb*float64(b[i])
		o[i] = float32(x)
		n += x * x
	}
	if n > 0 {
		k := float32(1 / math.Sqrt(n))
		for i := range o {
			o[i] *= k
		}
	}
	return o
}
