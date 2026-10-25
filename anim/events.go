package anim

import "math"

type Crossing struct {
	Event int32
	Cycle int64
	T     float64
}

const MaxCycles = 64

type walk struct {
	c     *Clip
	loop  LoopMode
	l     float64
	t0    float64
	t1    float64
	ks    int64
	fwd   bool
	start bool
	out   []Crossing
}

// Crossings lists the events crossed from t0 to t1 (start includes t0) by time, cycle and index; ping-pong turns fire once.
func Crossings(c *Clip, loop LoopMode, t0, t1 float64, start bool, out []Crossing) []Crossing {
	out = out[:0]
	l := float64(c.Length)
	if !(l > 0) || len(c.Events) == 0 || t0 != t0 || t1 != t1 {
		return out
	}
	var k0, k1, ks int64
	if loop == LoopOnce {
		t0, t1 = min(max(t0, 0), l), min(max(t1, 0), l)
	} else {
		ks = int64(math.Floor(t0 / l))
		k0 = int64(math.Floor(min(t0, t1)/l)) - 1
		k1 = int64(math.Floor(max(t0, t1) / l))
	}
	if t0 == t1 && !start {
		return out
	}
	fwd := t1 >= t0
	if k1-k0 > MaxCycles {
		if fwd {
			k0 = k1 - MaxCycles
		} else {
			k1 = k0 + MaxCycles
		}
	}
	w := walk{c: c, loop: loop, l: l, t0: t0, t1: t1, ks: ks, fwd: fwd, start: start, out: out}
	for j := int64(0); j <= k1-k0; j++ {
		k := k0 + j
		if !fwd {
			k = k1 - j
		}
		w.cycle(k)
	}
	return w.out
}

func (w *walk) cycle(k int64) {
	odd := w.loop == LoopPingPong && k&1 != 0
	ev := w.c.Events
	if w.fwd != odd {
		for i := range ev {
			w.try(k, odd, int32(i))
		}
		return
	}
	for i := len(ev) - 1; i >= 0; {
		g := i
		for g > 0 && ev[g-1].T == ev[i].T {
			g--
		}
		for j := g; j <= i; j++ {
			w.try(k, odd, int32(j))
		}
		i = g - 1
	}
}

func (w *walk) try(k int64, odd bool, i int32) {
	t := float64(w.c.Events[i].T)
	if !(t >= 0 && t <= w.l) {
		return
	}
	var tau float64
	switch {
	case w.loop == LoopOnce:
		tau = t
	case odd:
		if t == 0 || t == w.l {
			return
		}
		tau = float64(k)*w.l + w.l - t
	default:
		tau = float64(k)*w.l + t
	}
	at := w.start && tau == w.t0 && k == w.ks
	if w.fwd {
		if !(at || w.t0 < tau) || tau > w.t1 {
			return
		}
	} else if !(at || tau < w.t0) || tau < w.t1 {
		return
	}
	w.out = append(w.out, Crossing{Event: i, Cycle: k, T: tau})
}
