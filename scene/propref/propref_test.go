package propref

import "testing"

func TestLaneOf(t *testing.T) {
	cases := map[string]int8{"x": 0, "r": 0, "y": 1, "g": 1, "z": 2, "b": 2, "w": 3, "a": 3, "xy": -1, "q": -1, "": -1}
	for s, want := range cases {
		if got := laneOf(s); got != want {
			t.Errorf("laneOf(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestSameOverlapsHash(t *testing.T) {
	whole := Ref{own: OwnTransform, shape: Vec2, lane: -1}
	x, y := whole, whole
	x.lane, y.lane = 0, 1
	if !whole.Overlaps(&x) || !x.Overlaps(&whole) {
		t.Fatal("a lane must overlap the whole value")
	}
	if x.Overlaps(&y) {
		t.Fatal("different lanes must not overlap")
	}
	if !x.Same(&x) || x.Same(&y) || x.Same(&whole) {
		t.Fatal("Same must compare lanes")
	}
	if x.Hash() != whole.Hash() || y.Hash() != whole.Hash() {
		t.Fatal("Hash must not depend on the lane")
	}
	scale := whole
	scale.part = 2
	if scale.Overlaps(&whole) || scale.Hash() == whole.Hash() {
		t.Fatal("position and scale are different properties")
	}
	named := Ref{own: OwnScript, shape: Float, lane: -1}
	named.field.Field = "Speed"
	other := named
	other.field.Field = "Spin"
	if named.Overlaps(&other) || named.Hash() == other.Hash() {
		t.Fatal("script fields must be told apart by name")
	}
}

func TestLanesAndFits(t *testing.T) {
	v := Ref{shape: Vec3, lane: -1}
	if v.Lanes() != 3 || !v.Fits(Vec3) || v.Fits(Float) {
		t.Fatal("whole Vec3")
	}
	v.lane = 2
	if v.Lanes() != 1 || !v.Fits(Float) || v.Fits(Vec3) {
		t.Fatal("Vec3 lane")
	}
}

func TestTransformRefAndLane(t *testing.T) {
	p := Transform(false, PartPosition)
	if p.Shape() != Vec2 || p.Lane() != -1 {
		t.Fatal("2D position")
	}
	x := p.WithLane(0)
	if !x.Overlaps(&p) || x.Same(&p) || x.Hash() != p.Hash() {
		t.Fatal("lane of position")
	}
	if r := Transform(true, PartRotation); r.Shape() != Quat {
		t.Fatal("3D rotation")
	}
	d := Ref{shape: Discrete, lane: -1}
	if d.Fits(Float) || d.Fits(Discrete) {
		t.Fatal("discrete never fits a tween")
	}
}
