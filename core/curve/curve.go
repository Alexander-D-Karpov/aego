package curve

import (
	"cmp"
	"math"
	"slices"

	"aego/core/mathx"
)

type Interp uint8

const (
	InterpBezier Interp = iota
	InterpLinear
	InterpStep
)

type Tangent uint8

const (
	TangentAuto Tangent = iota
	TangentLinear
	TangentFree
)

type Wrap uint8

const (
	WrapClamp Wrap = iota
	WrapLoop
	WrapPingPong
)

var (
	interpNames  = []string{"bezier", "linear", "step"}
	tangentNames = []string{"auto", "linear", "free"}
	wrapNames    = []string{"clamp", "loop", "pingPong"}
)

func name(list []string, i uint8) string {
	if int(i) < len(list) {
		return list[i]
	}
	return list[0]
}

func parse(list []string, s string) (uint8, bool) {
	for i, n := range list {
		if n == s {
			return uint8(i), true
		}
	}
	return 0, false
}

func (i Interp) String() string  { return name(interpNames, uint8(i)) }
func (t Tangent) String() string { return name(tangentNames, uint8(t)) }
func (w Wrap) String() string    { return name(wrapNames, uint8(w)) }

func ParseInterp(s string) (Interp, bool) {
	v, ok := parse(interpNames, s)
	return Interp(v), ok
}

func ParseTangent(s string) (Tangent, bool) {
	v, ok := parse(tangentNames, s)
	return Tangent(v), ok
}

func ParseWrap(s string) (Wrap, bool) {
	v, ok := parse(wrapNames, s)
	return Wrap(v), ok
}

type Key struct {
	T       float32
	V       float32
	In      float32
	Out     float32
	Interp  Interp
	Tangent Tangent
}

type Curve struct {
	Ease Easing
	Keys []Key
	Pre  Wrap
	Post Wrap
}

func Constant(v float32) Curve {
	return Curve{Keys: []Key{{V: v}}}
}

func Line(t0, v0, t1, v1 float32) Curve {
	return Keyed(Key{T: t0, V: v0, Tangent: TangentLinear}, Key{T: t1, V: v1, Tangent: TangentLinear})
}

func FromEasing(e Easing) Curve {
	return Curve{Ease: e}
}

func Keyed(keys ...Key) Curve {
	c := Curve{Keys: slices.Clone(keys)}
	c.Fix()
	return c
}

func (c Curve) Clone() Curve {
	c.Keys = slices.Clone(c.Keys)
	return c
}

func (c Curve) Equal(o Curve) bool {
	return c.Ease == o.Ease && c.Pre == o.Pre && c.Post == o.Post && slices.Equal(c.Keys, o.Keys)
}

func (c *Curve) Fix() {
	slices.SortStableFunc(c.Keys, func(a, b Key) int { return cmp.Compare(a.T, b.T) })
	out := c.Keys[:0]
	for _, k := range c.Keys {
		if k.T != k.T || k.V != k.V {
			continue
		}
		if n := len(out); n > 0 && out[n-1].T == k.T {
			out[n-1] = k
			continue
		}
		out = append(out, k)
	}
	clear(c.Keys[len(out):])
	c.Keys = out
	c.Smooth()
}

func secant(a, b *Key) float32 {
	if d := b.T - a.T; d > 0 {
		return (b.V - a.V) / d
	}
	return 0
}

func auto(l, r float32) float32 {
	if l*r <= 0 {
		return 0
	}
	s := (l + r) / 2
	if lim := 3 * min(mathx.Abs(l), mathx.Abs(r)); mathx.Abs(s) > lim {
		return mathx.Sign(s) * lim
	}
	return s
}

func (c *Curve) Smooth() {
	ks := c.Keys
	n := len(ks)
	for i := range ks {
		k := &ks[i]
		if k.Tangent == TangentFree {
			continue
		}
		hasL, hasR := i > 0, i < n-1
		var l, r float32
		if hasL {
			l = secant(&ks[i-1], k)
		}
		if hasR {
			r = secant(k, &ks[i+1])
		}
		switch {
		case !hasL && !hasR:
			k.In, k.Out = 0, 0
		case !hasL:
			k.In, k.Out = r, r
		case !hasR:
			k.In, k.Out = l, l
		case k.Tangent == TangentLinear:
			k.In, k.Out = l, r
		default:
			s := auto(l, r)
			k.In, k.Out = s, s
		}
	}
}

func (c *Curve) Span() (float32, float32) {
	if c.Ease != None || len(c.Keys) == 0 {
		return 0, 1
	}
	return c.Keys[0].T, c.Keys[len(c.Keys)-1].T
}

func (c *Curve) Duration() float32 {
	t0, t1 := c.Span()
	return t1 - t0
}

func (c *Curve) Eval(t float32) float32 {
	t0, t1 := c.Span()
	switch {
	case t != t:
		t = t0
	case t < t0:
		t = wrap(t, t0, t1, c.Pre)
	case t > t1:
		t = wrap(t, t0, t1, c.Post)
	}
	return c.at(t)
}

func (c *Curve) EvalClamped(t float32) float32 {
	t0, t1 := c.Span()
	if t != t {
		t = t0
	}
	return c.at(mathx.Clamp(t, t0, t1))
}

func wrap(t, t0, t1 float32, w Wrap) float32 {
	d := float64(t1 - t0)
	if d <= 0 || w == WrapClamp {
		return mathx.Clamp(t, t0, t1)
	}
	period := d
	if w == WrapPingPong {
		period = 2 * d
	}
	m := math.Mod(float64(t-t0), period)
	if m < 0 {
		m += period
	}
	if m > d {
		m = 2*d - m
	}
	return t0 + float32(m)
}

func (c *Curve) at(t float32) float32 {
	if c.Ease != None {
		return Ease(c.Ease, t)
	}
	ks := c.Keys
	n := len(ks)
	switch {
	case n == 0:
		return 0
	case t <= ks[0].T:
		return ks[0].V
	case t >= ks[n-1].T:
		return ks[n-1].V
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
	return segment(&ks[lo], &ks[hi], t)
}

func segment(a, b *Key, t float32) float32 {
	d := b.T - a.T
	if d <= 0 {
		return b.V
	}
	switch a.Interp {
	case InterpStep:
		return a.V
	case InterpLinear:
		return a.V + (b.V-a.V)*(t-a.T)/d
	}
	u := (t - a.T) / d
	u2 := u * u
	u3 := u2 * u
	h00 := 2*u3 - 3*u2 + 1
	h10 := u3 - 2*u2 + u
	h01 := 3*u2 - 2*u3
	h11 := u3 - u2
	return h00*a.V + h10*d*a.Out + h01*b.V + h11*d*b.In
}

func (c *Curve) Normalize() {
	if c.Ease != None || len(c.Keys) < 2 {
		return
	}
	t0, t1 := c.Span()
	d := t1 - t0
	if d <= 0 {
		return
	}
	for i := range c.Keys {
		k := &c.Keys[i]
		k.T = (k.T - t0) / d
		k.In *= d
		k.Out *= d
	}
	c.Keys[0].T = 0
	c.Keys[len(c.Keys)-1].T = 1
}

func (c *Curve) Range(samples int) (lo, hi float32) {
	t0, t1 := c.Span()
	samples = max(samples, 2)
	lo, hi = mathx.Inf(), -mathx.Inf()
	for i := 0; i < samples; i++ {
		v := c.at(t0 + (t1-t0)*float32(i)/float32(samples-1))
		lo, hi = min(lo, v), max(hi, v)
	}
	for _, k := range c.Keys {
		lo, hi = min(lo, k.V), max(hi, k.V)
	}
	return lo, hi
}

func (c *Curve) Bake(samples int) Curve {
	if c.Ease == None {
		return c.Clone()
	}
	samples = max(samples, 2)
	const h = 1e-3
	out := Curve{Pre: c.Pre, Post: c.Post, Keys: make([]Key, samples)}
	for i := range out.Keys {
		t := float32(i) / float32(samples-1)
		a, b := max(t-h, 0), min(t+h, 1)
		s := (Ease(c.Ease, b) - Ease(c.Ease, a)) / (b - a)
		out.Keys[i] = Key{T: t, V: Ease(c.Ease, t), In: s, Out: s, Tangent: TangentFree}
	}
	return out
}
