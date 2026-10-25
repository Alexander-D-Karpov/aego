package anim

import (
	"math"
	"testing"

	"aego/core/curve"
	"aego/core/ids"
	"aego/schema"
)

func near(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-4
}

func one(kind schema.Kind, keys ...Key) *Clip {
	c := &Clip{Length: 1, Tracks: []Track{{Kind: kind, Keys: keys}}}
	c.Fix()
	return c
}

func TestLinearAndBezier(t *testing.T) {
	var s Sampler
	lin := one(schema.KindFloat32, Key{T: 0, V: Float(0), Interp: curve.InterpLinear}, Key{T: 1, V: Float(10)})
	if v := s.Track(lin, 0, 0.25); !near(v.L[0], 2.5) {
		t.Fatalf("linear %v", v.L[0])
	}
	bez := one(schema.KindFloat32, Key{T: 0, V: Float(0)}, Key{T: 1, V: Float(10)})
	if v := s.Track(bez, 0, 0.5); !near(v.L[0], 5) {
		t.Fatalf("two-key bezier must be linear, got %v", v.L[0])
	}
	if v := s.Track(bez, 0, -1); v.L[0] != 0 {
		t.Fatal("before the first key")
	}
	if v := s.Track(bez, 0, 2); v.L[0] != 10 {
		t.Fatal("after the last key")
	}
}

func TestStepAndDiscrete(t *testing.T) {
	var s Sampler
	step := one(schema.KindVec2, Key{T: 0, V: Lanes([4]float32{1, 2}), Interp: curve.InterpStep}, Key{T: 1, V: Lanes([4]float32{5, 6})})
	if v := s.Track(step, 0, 0.9); v.L != [4]float32{1, 2} {
		t.Fatalf("step %v", v.L)
	}
	b := one(schema.KindBool, Key{T: 0, V: Bool(false)}, Key{T: 0.5, V: Bool(true)})
	if s.Track(b, 0, 0.4).Truth() || !s.Track(b, 0, 0.6).Truth() {
		t.Fatal("bool steps")
	}
	a, z := ids.AssetID{Lo: 1}, ids.AssetID{Lo: 2}
	ar := one(schema.KindAssetRef, Key{T: 0, V: Asset(a)}, Key{T: 0.5, V: Asset(z)})
	if s.Track(ar, 0, 0.49).A != a || s.Track(ar, 0, 0.5).A != z {
		t.Fatal("asset steps")
	}
}

func TestInt(t *testing.T) {
	var s Sampler
	c := one(schema.KindInt32, Key{T: 0, V: Int(0)}, Key{T: 1, V: Int(10)})
	if s.Track(c, 0, 0.9).I != 0 {
		t.Fatal("int steps by default")
	}
	c.Tracks[0].Numeric = true
	if v := s.Track(c, 0, 0.26).I; v != 3 {
		t.Fatalf("numeric int rounds, got %d", v)
	}
}

func TestQuatSlerp(t *testing.T) {
	var s Sampler
	h := float32(math.Sqrt2 / 2)
	c := one(schema.KindQuat, Key{T: 0, V: Lanes([4]float32{0, 0, 0, 1})}, Key{T: 1, V: Lanes([4]float32{0, 0, h, h})})
	v := s.Track(c, 0, 0.5)
	if !near(v.L[2], 0.38268) || !near(v.L[3], 0.92388) {
		t.Fatalf("slerp %v", v.L)
	}
}

func TestLocal(t *testing.T) {
	c := &Clip{Length: 1}
	if u, k := c.Local(LoopRepeat, 2.25); !near(u, 0.25) || k != 2 {
		t.Fatalf("repeat %v %d", u, k)
	}
	if u, k := c.Local(LoopPingPong, 1.25); !near(u, 0.75) || k != 1 {
		t.Fatalf("pingpong %v %d", u, k)
	}
	if u, k := c.Local(LoopOnce, 3); u != 1 || k != 0 {
		t.Fatalf("once %v %d", u, k)
	}
}

func TestCheck(t *testing.T) {
	c := &Clip{Length: 1, Tracks: []Track{
		{Kind: schema.KindString, Keys: []Key{{T: 0}}},
		{Kind: schema.KindFloat32, Keys: []Key{{T: 2}}},
	}, Events: []Event{{T: 0.5}}}
	got := c.Check(nil)
	want := []ProblemKind{ProblemBadKind, ProblemKeyRange, ProblemEventName}
	if len(got) != len(want) {
		t.Fatalf("problems %+v", got)
	}
	for i := range want {
		if got[i].Kind != want[i] {
			t.Fatalf("problem %d: %+v", i, got[i])
		}
	}
}

func clip1000() *Clip {
	c := &Clip{Length: 1, Tracks: make([]Track, 1000)}
	for i := range c.Tracks {
		c.Tracks[i] = Track{Kind: schema.KindVec2, Keys: []Key{{T: 0}, {T: 0.5, V: Lanes([4]float32{1, 2})}, {T: 1}}}
	}
	c.Fix()
	return c
}

func TestSampleNoAllocs(t *testing.T) {
	c := clip1000()
	var s Sampler
	s.Reset(c)
	out := make([]Value, len(c.Tracks))
	if n := testing.AllocsPerRun(100, func() { s.Sample(c, 0.3, out) }); n != 0 {
		t.Fatalf("Sample allocates %v", n)
	}
}

func BenchmarkSample1000(b *testing.B) {
	c := clip1000()
	var s Sampler
	s.Reset(c)
	out := make([]Value, len(c.Tracks))
	b.ReportAllocs()
	t := float32(0)
	for b.Loop() {
		t += 0.016
		if t > 1 {
			t = 0
		}
		s.Sample(c, t, out)
	}
}
