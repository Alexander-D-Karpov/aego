package curve

import (
	"testing"

	"aego/core/mathx"
)

func near(t *testing.T, got, want float32, what string) {
	t.Helper()
	if !mathx.NearlyEqual(got, want, 1e-5) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

func TestEaseEndpoints(t *testing.T) {
	for e := Linear; e < EasingCount; e++ {
		near(t, Ease(e, 0), 0, e.String()+"(0)")
		near(t, Ease(e, 1), 1, e.String()+"(1)")
		if p, ok := ParseEasing(e.String()); !ok || p != e {
			t.Fatalf("%s does not round-trip its name", e)
		}
	}
}

func TestEaseAnalytic(t *testing.T) {
	near(t, Ease(InQuad, 0.5), 0.25, "inQuad")
	near(t, Ease(OutQuad, 0.5), 0.75, "outQuad")
	near(t, Ease(OutCubic, 0.5), 0.875, "outCubic")
	near(t, Ease(InOutSine, 0.5), 0.5, "inOutSine")
	near(t, Ease(InOutQuad, 0.25), 0.125, "inOutQuad")
	near(t, Ease(OutBounce, 1/2.75), 1, "outBounce")
	if Ease(OutBack, 0.6) <= 1 {
		t.Fatal("outBack must overshoot")
	}
}

func TestAutoDoesNotOvershoot(t *testing.T) {
	c := Keyed(Key{T: 0, V: 0}, Key{T: 1, V: 1}, Key{T: 2, V: 1}, Key{T: 3, V: 0}, Key{T: 3.2, V: 5})
	for i := 0; i <= 320; i++ {
		x := float32(i) / 100
		v := c.Eval(x)
		lo, hi := float32(0), float32(1)
		if x > 3 {
			hi = 5
		}
		if v < lo-1e-5 || v > hi+1e-5 {
			t.Fatalf("value %v at %v leaves [%v, %v]", v, x, lo, hi)
		}
	}
}

func TestInterpModes(t *testing.T) {
	c := Keyed(Key{T: 0, V: 0, Interp: InterpLinear}, Key{T: 2, V: 4, Interp: InterpStep}, Key{T: 4, V: 0})
	near(t, c.Eval(1), 2, "linear")
	near(t, c.Eval(3), 4, "step")
	near(t, c.Eval(4), 0, "last key")
	near(t, c.Eval(-1), 0, "clamped before")
}

func TestWrap(t *testing.T) {
	c := Line(0, 0, 1, 1)
	c.Post = WrapLoop
	near(t, c.Eval(1.25), 0.25, "loop")
	c.Post = WrapPingPong
	near(t, c.Eval(1.25), 0.75, "pingPong")
	c.Pre = WrapPingPong
	near(t, c.Eval(-0.25), 0.25, "pre pingPong")
	near(t, c.EvalClamped(1.25), 1, "clamped")
}

func TestFixSortsAndDeduplicates(t *testing.T) {
	c := Keyed(Key{T: 2, V: 1}, Key{T: 0, V: 0}, Key{T: 2, V: 3})
	if len(c.Keys) != 2 || c.Keys[1].V != 3 {
		t.Fatalf("keys %+v", c.Keys)
	}
}

func TestNormalizeKeepsShape(t *testing.T) {
	c := Keyed(Key{T: 2, V: 0}, Key{T: 4, V: 3}, Key{T: 6, V: 1})
	n := c.Clone()
	n.Normalize()
	for i := 0; i <= 20; i++ {
		x := 2 + 4*float32(i)/20
		near(t, n.Eval((x-2)/4), c.Eval(x), "normalized")
	}
}

func TestBakeFollowsPreset(t *testing.T) {
	c := FromEasing(InOutCubic)
	b := c.Bake(17)
	for i := 0; i <= 50; i++ {
		x := float32(i) / 50
		if !mathx.NearlyEqual(b.Eval(x), c.Eval(x), 2e-3) {
			t.Fatalf("baked %v differs at %v: %v", b.Eval(x), x, c.Eval(x))
		}
	}
}

func TestEvalAllocs(t *testing.T) {
	c := Keyed(Key{T: 0, V: 0}, Key{T: 1, V: 2}, Key{T: 3, V: 1})
	if n := testing.AllocsPerRun(100, func() { _ = c.Eval(1.7) }); n != 0 {
		t.Fatalf("Eval allocates %v", n)
	}
}
