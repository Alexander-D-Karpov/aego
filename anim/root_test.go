package anim

import (
	"testing"

	"aego/core/curve"
	"aego/schema"
)

func walkClip(loop LoopMode) *Clip {
	c := &Clip{Length: 1, Loop: loop, Tracks: []Track{{
		Property: RootProp, Kind: schema.KindVec2, Root: true,
		Keys: []Key{{T: 0, V: Lanes([4]float32{0, 0}), Interp: curve.InterpLinear}, {T: 1, V: Lanes([4]float32{1, 0.5})}},
	}}}
	c.Fix()
	return c
}

func TestRootDelta(t *testing.T) {
	var s Sampler
	loop := walkClip(LoopRepeat)
	for _, tc := range []struct {
		t0, t1 float64
		want   [2]float32
	}{
		{0.25, 0.75, [2]float32{0.5, 0.25}},
		{0.5, 1.5, [2]float32{1, 0.5}},
		{0.25, 2.25, [2]float32{2, 1}},
		{1.25, 0.25, [2]float32{-1, -0.5}},
		{0.75, 1, [2]float32{0.25, 0.125}},
	} {
		d := RootDelta(loop, &s, 0, LoopRepeat, tc.t0, tc.t1)
		if !near(d[0], tc.want[0]) || !near(d[1], tc.want[1]) {
			t.Fatalf("loop %v..%v: %v, want %v", tc.t0, tc.t1, d, tc.want)
		}
	}
	ping := walkClip(LoopPingPong)
	if d := RootDelta(ping, &s, 0, LoopPingPong, 0.5, 1.5); !near(d[0], 0) || !near(d[1], 0) {
		t.Fatalf("ping-pong turns back: %v", d)
	}
	once := walkClip(LoopOnce)
	if d := RootDelta(once, &s, 0, LoopOnce, 0.75, 3); !near(d[0], 0.25) {
		t.Fatalf("once stops at the end: %v", d)
	}
	if d := RootDelta(loop, &s, 0, LoopRepeat, 0.5, 0.5); d != ([2]float32{}) {
		t.Fatalf("no time, no motion: %v", d)
	}
}

func TestRootCheck(t *testing.T) {
	c := walkClip(LoopRepeat)
	if ps := c.Check(nil); len(ps) != 0 {
		t.Fatalf("a good root track reports %+v", ps)
	}
	c.Tracks[0].Target = "Body"
	if ps := c.Check(nil); len(ps) != 1 || ps[0].Kind != ProblemRoot {
		t.Fatalf("a root track on a child: %+v", ps)
	}
}
