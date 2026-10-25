package anim

import (
	"cmp"
	"math"
	"slices"

	"aego/core/curve"
	"aego/core/mathx"
	"aego/schema"
)

type LoopMode uint8

const (
	LoopOnce LoopMode = iota
	LoopRepeat
	LoopPingPong
)

type Authority uint8

const (
	AuthPresentation Authority = iota
	AuthSimulation
)

type OnEnd uint8

const (
	EndHold OnEnd = iota
	EndRestore
)

type Mode uint8

const (
	EventGameplay Mode = iota
	EventPresentation
	EventPreview
)

type Key struct {
	T       float32
	V       Value
	In      [4]float32
	Out     [4]float32
	Interp  curve.Interp
	Tangent curve.Tangent
}

type Track struct {
	Target    string
	Property  string
	Kind      schema.Kind
	Numeric   bool
	Authority Authority
	OnEnd     OnEnd
	Root      bool
	Keys      []Key
}

const RootProp = "transform.position"

type Event struct {
	T       float32
	Name    string
	ID      int32
	Payload float32
	Mode    Mode
}

type Clip struct {
	Length float32
	FPS    float32
	Loop   LoopMode
	Tracks []Track
	Events []Event
}

func (c *Clip) Fix() {
	for i := range c.Tracks {
		c.Tracks[i].fix()
	}
	slices.SortStableFunc(c.Events, func(a, b Event) int { return cmp.Compare(a.T, b.T) })
}

func (t *Track) fix() {
	slices.SortStableFunc(t.Keys, func(a, b Key) int { return cmp.Compare(a.T, b.T) })
	out := t.Keys[:0]
	for _, k := range t.Keys {
		if k.T != k.T {
			continue
		}
		if n := len(out); n > 0 && out[n-1].T == k.T {
			out[n-1] = k
			continue
		}
		out = append(out, k)
	}
	clear(t.Keys[len(out):])
	t.Keys = out
	if classOf(t.Kind) == classLanes {
		t.smooth()
	}
}

func secant(a, b *Key, l int) float32 {
	if d := b.T - a.T; d > 0 {
		return (b.V.L[l] - a.V.L[l]) / d
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

func (t *Track) smooth() {
	ks := t.Keys
	n := len(ks)
	lanes := lanesOf(t.Kind)
	for i := range ks {
		k := &ks[i]
		if k.Tangent == curve.TangentFree {
			continue
		}
		hasL, hasR := i > 0, i < n-1
		for l := 0; l < lanes; l++ {
			var a, b, in, out float32
			if hasL {
				a = secant(&ks[i-1], k, l)
			}
			if hasR {
				b = secant(k, &ks[i+1], l)
			}
			switch {
			case !hasL && !hasR:
			case !hasL:
				in, out = b, b
			case !hasR:
				in, out = a, a
			case k.Tangent == curve.TangentLinear:
				in, out = a, b
			default:
				s := auto(a, b)
				in, out = s, s
			}
			k.In[l], k.Out[l] = in, out
		}
	}
}

func (c *Clip) Local(loop LoopMode, t float64) (float32, int64) {
	l := float64(c.Length)
	if !(l > 0) || t != t {
		return 0, 0
	}
	if loop == LoopOnce {
		return float32(min(max(t, 0), l)), 0
	}
	k := math.Floor(t / l)
	u := t - k*l
	if u >= l {
		u, k = 0, k+1
	}
	u = max(u, 0)
	cycle := int64(k)
	if loop == LoopPingPong && cycle&1 != 0 {
		u = l - u
	}
	return float32(u), cycle
}

type ProblemKind uint8

const (
	ProblemLength ProblemKind = iota
	ProblemBadKind
	ProblemEmpty
	ProblemKeyRange
	ProblemEventRange
	ProblemEventName
	ProblemRoot
)

type Problem struct {
	Kind  ProblemKind
	Track int
	Event int
	T     float32
}

func (c *Clip) Check(dst []Problem) []Problem {
	if !(c.Length > 0) {
		dst = append(dst, Problem{Kind: ProblemLength, Track: -1, Event: -1, T: c.Length})
	}
	for i := range c.Tracks {
		t := &c.Tracks[i]
		switch {
		case !Supported(t.Kind):
			dst = append(dst, Problem{Kind: ProblemBadKind, Track: i, Event: -1})
			continue
		case len(t.Keys) == 0:
			dst = append(dst, Problem{Kind: ProblemEmpty, Track: i, Event: -1})
			continue
		}
		if t.Root && (t.Target != "" || t.Property != RootProp || t.Kind != schema.KindVec2) {
			dst = append(dst, Problem{Kind: ProblemRoot, Track: i, Event: -1})
		}
		for _, k := range t.Keys {
			if k.T < 0 || k.T > c.Length {
				dst = append(dst, Problem{Kind: ProblemKeyRange, Track: i, Event: -1, T: k.T})
				break
			}
		}
	}
	for i, e := range c.Events {
		switch {
		case !(e.T >= 0 && e.T <= c.Length):
			dst = append(dst, Problem{Kind: ProblemEventRange, Track: -1, Event: i, T: e.T})
		case e.Name == "":
			dst = append(dst, Problem{Kind: ProblemEventName, Track: -1, Event: i, T: e.T})
		}
	}
	return dst
}
