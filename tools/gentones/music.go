package main

import "math"

const bpm = 112.0

type chord struct {
	root  int
	notes [3]int
}

type tone struct {
	bar      int
	at, dur  float64
	key      int
	velocity float64
}

var changes = [8]chord{
	{45, [3]int{57, 60, 64}},
	{41, [3]int{57, 60, 65}},
	{48, [3]int{55, 60, 64}},
	{43, [3]int{55, 59, 62}},
	{45, [3]int{57, 60, 64}},
	{41, [3]int{57, 60, 65}},
	{38, [3]int{57, 62, 65}},
	{40, [3]int{56, 59, 64}},
}

var tune = []tone{
	{8, 0, 1.5, 76, 1}, {8, 1.5, 0.5, 74, 0.8}, {8, 2, 1, 72, 0.9}, {8, 3, 1, 74, 0.8},
	{9, 0, 2, 76, 1}, {9, 2, 1, 69, 0.8}, {9, 3, 1, 72, 0.85},
	{10, 0, 1.5, 79, 1}, {10, 1.5, 0.5, 76, 0.8}, {10, 2, 1, 72, 0.85}, {10, 3, 1, 76, 0.85},
	{11, 0, 3, 74, 1}, {11, 3, 1, 71, 0.8},
	{12, 0, 1, 72, 0.9}, {12, 1, 1, 76, 0.9}, {12, 2, 1.5, 81, 1}, {12, 3.5, 0.5, 79, 0.8},
	{13, 0, 1.5, 77, 1}, {13, 1.5, 0.5, 76, 0.8}, {13, 2, 2, 72, 0.9},
	{14, 0, 1, 74, 0.9}, {14, 1, 1, 77, 0.9}, {14, 2, 1, 76, 0.9}, {14, 3, 1, 74, 0.85},
	{15, 0, 1, 68, 0.85}, {15, 1, 1, 71, 0.9}, {15, 2, 1, 76, 1}, {15, 3, 0.5, 74, 0.8}, {15, 3.5, 0.5, 71, 0.8},
	{0, 0, 2.5, 69, 0.9},
}

var arpOrder = [8]int{0, 1, 2, 3, 1, 2, 3, 2}

// song renders into loop-length buses; every voice wraps past the end, so the loop has no seam.
type song struct {
	n                         int
	beat                      float64
	drums, bass, pad, keys    []float64
	lead, duck, delay, reverb []float64
	rnd                       noise
}

func music() []float32 {
	s := &song{beat: 60 / bpm, rnd: noise(0xbeef)}
	s.n = int(math.Round(16 * 4 * s.beat * rate))
	for _, b := range []*[]float64{&s.drums, &s.bass, &s.pad, &s.keys, &s.lead, &s.duck, &s.delay, &s.reverb} {
		*b = make([]float64, s.n)
	}
	for i := range s.duck {
		s.duck[i] = 1
	}
	for bar := range 16 {
		c := changes[bar%8]
		s.drumBar(bar)
		s.bassBar(bar, c.root)
		for _, k := range c.notes {
			s.padNote(s.at(bar, 0), midi(k), 4*s.beat+0.35, 0.11)
		}
		for step, o := range arpOrder {
			k := c.notes[min(o, 2)]
			if o == 3 {
				k = c.notes[0] + 12
			}
			v := 0.75
			if step%2 == 0 {
				v = 1
			}
			s.pluck(s.at(bar, float64(step)*0.5), midi(k), 0.9, 0.42*v)
		}
	}
	for _, t := range tune {
		s.leadNote(s.at(t.bar, t.at), midi(t.key), t.dur*s.beat, 0.17*t.velocity)
	}
	lowpass(s.pad, 1400, 2)
	mix := make([]float64, s.n)
	echo := feedbackDelay(s.delay, int(0.75*s.beat*rate), 0.38, 2600)
	room := freeverb(s.reverb)
	for i := range mix {
		mix[i] = s.drums[i] + s.duck[i]*(s.bass[i]+s.pad[i]) + s.keys[i] + s.lead[i] + 0.5*echo[i] + 2.4*room[i]
	}
	return master(mix)
}

func (s *song) at(bar int, beat float64) int {
	return int((float64(bar*4) + beat) * s.beat * rate)
}

func (s *song) add(buf []float64, i int, v float64) {
	buf[(i%s.n+s.n)%s.n] += v
}

func (s *song) drumBar(bar int) {
	kicks := []float64{0, 2}
	if bar%2 == 1 {
		kicks = append(kicks, 2.75)
	}
	for _, b := range kicks {
		s.kick(s.at(bar, b))
	}
	s.snare(s.at(bar, 1), 1)
	s.snare(s.at(bar, 3), 1)
	if bar%8 == 7 {
		for _, b := range []float64{3.25, 3.5, 3.75} {
			s.snare(s.at(bar, b), 0.45+0.15*(b-3)*4)
		}
	}
	for e := range 8 {
		v, decay := 0.55, 0.035
		if e%2 == 1 {
			v = 1
		}
		if e == 7 && bar%2 == 1 {
			v, decay = 0.9, 0.16
		}
		s.cymbal(s.at(bar, float64(e)*0.5), v, decay, 7000, 0.1)
	}
}

func (s *song) kick(at int) {
	n := int(0.42 * rate)
	phase := 0.0
	for j := range n {
		t := float64(j) / rate
		phase += 2 * math.Pi * (48 + 115*math.Exp(-t/0.032)) / rate
		v := math.Sin(phase)*math.Exp(-t/0.2) + 0.25*s.rnd.next()*math.Exp(-t/0.002)
		s.add(s.drums, at+j, 0.5*v*edge(j, n, rate/250))
	}
	for j := range int(0.35 * rate) {
		t := float64(j) / rate
		d := 1 - 0.6*math.Exp(-t/0.12)*math.Min(t/0.004, 1)
		k := ((at+j)%s.n + s.n) % s.n
		s.duck[k] = math.Min(s.duck[k], d)
	}
}

func (s *song) snare(at int, v float64) {
	n := int(0.3 * rate)
	phase, hi, lo := 0.0, 0.0, 0.0
	ah, al := onePole(7000), onePole(1000)
	for j := range n {
		t := float64(j) / rate
		phase += 2 * math.Pi * (175 + 60*math.Exp(-t/0.01)) / rate
		hi += ah * (s.rnd.next() - hi)
		lo += al * (hi - lo)
		y := v * (0.45*math.Sin(phase)*math.Exp(-t/0.05) + 0.75*(hi-lo)*math.Exp(-t/0.11)) * edge(j, n, rate/1000)
		s.add(s.drums, at+j, 0.38*y)
		s.add(s.reverb, at+j, 0.25*y)
	}
}

var metal = [6]float64{205.3, 304.4, 369.6, 522.7, 540, 800}

// cymbal is six detuned square waves and noise through a three-pole high-pass, like an 808's metal circuit.
func (s *song) cymbal(at int, v, decay, cutoff, wet float64) {
	n := int((decay*5 + 0.01) * rate)
	var ph [6]float64
	a := onePole(cutoff)
	l1, l2, l3 := 0.0, 0.0, 0.0
	v *= 0.9 + 0.1*s.rnd.next()
	for j := range n {
		t := float64(j) / rate
		m := 0.0
		for k, f := range metal {
			dt := f / rate
			ph[k] += dt
			ph[k] -= math.Floor(ph[k])
			q := ph[k] + 0.5
			m += blepSaw(ph[k], dt) - blepSaw(q-math.Floor(q), dt)
		}
		x := m/6 + 0.5*s.rnd.next()
		l1 += a * (x - l1)
		h1 := x - l1
		l2 += a * (h1 - l2)
		h2 := h1 - l2
		l3 += a * (h2 - l3)
		y := 2 * v * (h2 - l3) * math.Exp(-t/decay) * math.Min(t/0.0015, 1) * edge(j, n, rate/2000)
		s.add(s.drums, at+j, y)
		s.add(s.reverb, at+j, wet*y)
	}
}

func (s *song) bassBar(bar, root int) {
	steps := []struct {
		at, dur float64
		oct     int
		v       float64
	}{{0, 0.45, 0, 1}, {0.5, 0.3, 0, 0.6}, {1.5, 0.45, 12, 0.8}, {2, 0.45, 0, 0.95}, {2.5, 0.3, 0, 0.6}, {3, 0.4, 0, 0.8}, {3.5, 0.45, 12, 0.75}}
	for _, st := range steps {
		s.bassNote(s.at(bar, st.at), midi(root+st.oct), st.dur*s.beat, 0.48*st.v)
	}
}

func (s *song) bassNote(at int, f, dur, gain float64) {
	n := int((dur + 0.03) * rate)
	phase, lp1, lp2 := 0.0, 0.0, 0.0
	for j := range n {
		t := float64(j) / rate
		phase += f / rate
		phase -= math.Floor(phase)
		x := 0.6*blepSaw(phase, f/rate) + 0.7*math.Sin(2*math.Pi*phase)
		a := onePole(180 + 1500*math.Exp(-t/0.06))
		lp1 += a * (x - lp1)
		lp2 += a * (lp1 - lp2)
		env := math.Min(t/0.004, 1)
		if t > dur {
			env *= 1 - (t-dur)/0.03
		}
		s.add(s.bass, at+j, gain*lp2*env)
	}
}

func (s *song) padNote(at int, f, dur, gain float64) {
	n := int((dur + 0.6) * rate)
	var phases [3]float64
	detune := [3]float64{-0.006, 0, 0.0055}
	for j := range n {
		t := float64(j) / rate
		v := 0.0
		for k, d := range detune {
			fk := f * (1 + d)
			phases[k] += fk / rate
			phases[k] -= math.Floor(phases[k])
			v += blepSaw(phases[k], fk/rate)
		}
		env := math.Min(t/0.45, 1)
		if t > dur {
			env *= math.Exp(-(t - dur) / 0.18)
		}
		y := gain * v * env / 3
		s.add(s.pad, at+j, y)
		s.add(s.reverb, at+j, 0.5*y)
	}
}

// pluck is a Karplus–Strong string with a fractional delay, so high notes stay in tune.
func (s *song) pluck(at int, f, dur, gain float64) {
	n := int(dur * rate)
	period := rate/f - 0.5
	whole := int(period)
	frac := period - float64(whole)
	y := make([]float64, n)
	lp := 0.0
	for j := range n {
		x := 0.0
		if j < whole+1 {
			lp += 0.45 * (s.rnd.next() - lp)
			x = lp
		}
		if j > whole+1 {
			a := (1-frac)*y[j-whole] + frac*y[j-whole-1]
			b := (1-frac)*y[j-whole-1] + frac*y[j-whole-2]
			x += 0.996 * 0.5 * (a + b)
		}
		y[j] = x
		v := gain * x * edge(j, n, rate/50)
		s.add(s.keys, at+j, v)
		s.add(s.delay, at+j, 0.55*v)
		s.add(s.reverb, at+j, 0.3*v)
	}
}

func (s *song) leadNote(at int, f, dur, gain float64) {
	n := int((dur + 0.12) * rate)
	phase := 0.0
	for j := range n {
		t := float64(j) / rate
		vib := 1 + 0.004*math.Sin(2*math.Pi*5.2*t)*math.Min(math.Max(t-0.18, 0)/0.25, 1)
		phase += f * vib / rate
		p := 2 * math.Pi * phase
		x := math.Sin(p) + 0.18*math.Sin(2*p) + 0.1*math.Sin(3*p)
		env := math.Min(t/0.018, 1) * (0.8 + 0.2*math.Exp(-t/0.15))
		if t > dur {
			env *= 1 - (t-dur)/0.12
		}
		v := gain * x * env
		s.add(s.lead, at+j, v)
		s.add(s.delay, at+j, 0.4*v)
		s.add(s.reverb, at+j, 0.45*v)
	}
}

func blepSaw(p, dt float64) float64 {
	v := 2*p - 1
	switch {
	case p < dt:
		x := p / dt
		v -= x + x - x*x - 1
	case p > 1-dt:
		x := (p - 1) / dt
		v -= x*x + x + x + 1
	}
	return v
}

func onePole(cutoff float64) float64 {
	return 1 - math.Exp(-2*math.Pi*cutoff/rate)
}

// lowpass filters a loop in place; the first cycle warms the filter up, so the seam stays smooth.
func lowpass(buf []float64, cutoff float64, poles int) {
	a := onePole(cutoff)
	for range poles {
		y := 0.0
		for pass := range 2 {
			for i, x := range buf {
				y += a * (x - y)
				if pass == 1 {
					buf[i] = y
				}
			}
		}
	}
}

func feedbackDelay(in []float64, d int, fb, cutoff float64) []float64 {
	n := len(in)
	out := make([]float64, n)
	line := make([]float64, 2*n)
	a, lp := onePole(cutoff), 0.0
	for i := range 2 * n {
		back := 0.0
		if i >= d {
			back = line[i-d]
		}
		lp += a * (back - lp)
		line[i] = in[i%n] + fb*lp
		if i >= n {
			out[i-n] = lp
		}
	}
	return out
}

// freeverb is Jezar's Freeverb reduced to one channel, run for two cycles of the loop.
func freeverb(in []float64) []float64 {
	n := len(in)
	combs := []int{1557, 1617, 1491, 1422, 1277, 1356}
	allpasses := []int{556, 441, 341}
	out := make([]float64, n)
	type comb struct {
		buf    []float64
		i      int
		filter float64
	}
	cs := make([]comb, len(combs))
	for k, l := range combs {
		cs[k].buf = make([]float64, l*rate/44100)
	}
	aps := make([][]float64, len(allpasses))
	api := make([]int, len(allpasses))
	for k, l := range allpasses {
		aps[k] = make([]float64, l*rate/44100)
	}
	for i := range 2 * n {
		x := 0.015 * in[i%n]
		y := 0.0
		for k := range cs {
			c := &cs[k]
			o := c.buf[c.i]
			c.filter = o*0.65 + c.filter*0.35
			c.buf[c.i] = x + c.filter*0.84
			c.i = (c.i + 1) % len(c.buf)
			y += o
		}
		for k := range aps {
			b := aps[k]
			o := b[api[k]]
			b[api[k]] = y + o*0.5
			y = o - y
			api[k] = (api[k] + 1) % len(b)
		}
		if i >= n {
			out[i-n] = y
		}
	}
	return out
}

func master(mix []float64) []float32 {
	peak := 0.0
	for _, v := range mix {
		peak = math.Max(peak, math.Abs(v))
	}
	out := make([]float32, len(mix))
	if peak == 0 {
		return out
	}
	const drive = 1.6
	for i, v := range mix {
		out[i] = float32(0.9 * math.Tanh(drive*v/peak) / math.Tanh(drive))
	}
	return out
}
