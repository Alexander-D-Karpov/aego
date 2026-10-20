package camera

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
)

func near(a, b, eps float32) bool {
	return mathx.Abs(a-b) <= eps
}

func TestFollowInsideDeadzone(t *testing.T) {
	pos := mathx.Vec2{X: 5, Y: 2}
	got := followStep(pos, mathx.Vec2{X: 5.5, Y: 2.2}, mathx.Vec2{X: 20, Y: 10}, mathx.Rect{X: 0.4, Y: 0.4, W: 0.2, H: 0.2}, mathx.Vec2{X: 5, Y: 5}, 1.0/60)
	if got != pos {
		t.Fatalf("camera moved inside the deadzone: %v", got)
	}
}

func TestFollowConvergesAcrossRates(t *testing.T) {
	dz := mathx.Rect{X: 0.5, Y: 0.5}
	var res []float32
	for _, hz := range []float32{30, 60, 144} {
		dt := 1 / hz
		pos := mathx.Vec2{}
		for i := 0; i < int(2*hz+0.5); i++ {
			pos = followStep(pos, mathx.Vec2{X: 10}, mathx.Vec2{X: 20, Y: 10}, dz, mathx.Vec2{X: 5, Y: 5}, dt)
		}
		res = append(res, pos.X)
	}
	for i, r := range res {
		if !near(r, res[0], 1e-3) {
			t.Fatalf("rate %d ends at %v, 30 Hz at %v", i, r, res[0])
		}
	}
}

func TestFollowRigid(t *testing.T) {
	got := followStep(mathx.Vec2{}, mathx.Vec2{X: 3, Y: -2}, mathx.Vec2{X: 20, Y: 10}, mathx.Rect{X: 0.5, Y: 0.5}, mathx.Vec2{}, 0)
	if got != (mathx.Vec2{X: 3, Y: -2}) {
		t.Fatalf("damping 0 must follow rigidly, got %v", got)
	}
}

func TestClampAxis(t *testing.T) {
	cases := []struct{ c, lo, size, half, want float32 }{
		{0, -2, 3, 5, -0.5},
		{10, 0, 20, 5, 10},
		{1, 0, 20, 5, 5},
		{19, 0, 20, 5, 15},
	}
	for _, c := range cases {
		if got := clampAxis(c.c, c.lo, c.size, c.half); !near(got, c.want, 1e-6) {
			t.Fatalf("clampAxis(%v) = %v, want %v", c, got, c.want)
		}
	}
}

func TestNoiseDeterministic(t *testing.T) {
	seed := seedOf("main")
	varied := false
	prev := noise1(seed, 0)
	for i := 0; i < 1000; i++ {
		x := float32(i) * 0.137
		a, b := noise1(seed, x), noise1(seed, x)
		if a != b {
			t.Fatalf("noise differs at %v", x)
		}
		if a < -1 || a > 1 {
			t.Fatalf("noise %v out of range at %v", a, x)
		}
		if a != prev {
			varied = true
		}
		prev = a
	}
	if !varied {
		t.Fatal("noise is constant")
	}
	if noise1(seed^1, 3.3) == noise1(seed^2, 3.3) {
		t.Fatal("channels share a sequence")
	}
}

func TestLerpPose(t *testing.T) {
	p := lerpPose(Pose{Zoom: 16}, Pose{Zoom: 64}, 0.5)
	if !near(p.Zoom, 32, 1e-3) {
		t.Fatalf("zoom blends in log space: %v", p.Zoom)
	}
	r := lerpPose(Pose{Rotation: 3, Zoom: 1}, Pose{Rotation: -3, Zoom: 1}, 0.5)
	if !near(r.Rotation, 3.1416, 1e-3) {
		t.Fatalf("rotation takes the short arc: %v", r.Rotation)
	}
}

func TestCandidates(t *testing.T) {
	a := cand{vc: scene.NodeHandle{Index: 1, Gen: 1}}
	b := cand{vc: scene.NodeHandle{Index: 2, Gen: 1}}
	if !a.beats(b) || b.beats(a) {
		t.Fatal("equal priority: the lower handle index wins")
	}
	z1 := cand{vc: b.vc, kind: kindZone, seq: 1}
	z2 := cand{vc: a.vc, kind: kindZone, seq: 2}
	if !z1.beats(a) || !z2.beats(z1) {
		t.Fatal("zones beat equal base priorities, the later zone wins")
	}
	hi := cand{vc: b.vc, prio: 5}
	if !hi.beats(z2) {
		t.Fatal("higher priority wins over a zone")
	}
	f := cand{vc: b.vc, prio: 1 << 30, kind: kindForced}
	if !f.beats(hi) {
		t.Fatal("forced camera wins")
	}
}
