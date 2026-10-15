package world

import (
	"slices"
	"testing"
)

func collideGeoms(a *geom, xa xform, b *geom, xb xform) Manifold {
	var m Manifold
	ga, gb := a.c, b.c
	collideConvex(&ga, xa, &gb, xb, 0.02, &m)
	return m
}

func TestCircleCircle(t *testing.T) {
	c := gm(t, Circle{R: 1}, Transform{})
	for i := 0; i < 20; i++ {
		a := float32(i) * 0.31
		n := makeRot(a).apply(Vec2{X: 1})
		m := collideGeoms(&c, xfAt(0, 0, 0), &c, xfAt(1.5*n.X, 1.5*n.Y, a))
		if m.Count != 1 || !near(m.Points[0].Separation, -0.5, 1e-5) || !near2(m.Normal, n, 1e-5) || !near2(m.Points[0].Point, n.Scale(0.75), 1e-5) {
			t.Fatalf("pose %d: %+v", i, m)
		}
	}
}

func TestBoxBox(t *testing.T) {
	box := gm(t, Box{Half: Vec2{X: 1, Y: 1}}, Transform{})
	for i := 0; i < 20; i++ {
		th := float32(i) * 0.3
		x := -0.5 + float32(i)*0.05
		q := makeRot(th)
		pb := q.apply(Vec2{X: x, Y: 1.9})
		xa, xb := xfAt(0, 0, th), xfAt(pb.X, pb.Y, th)
		m := collideGeoms(&box, xa, &box, xb)
		if m.Count != 2 || !near2(m.Normal, q.apply(Vec2{Y: 1}), 1e-4) {
			t.Fatalf("pose %d: %+v", i, m)
		}
		want := []float32{max(-1, x-1), min(1, x+1)}
		for k := 0; k < 2; k++ {
			p := xa.invApply(m.Points[k].Point)
			if !near(m.Points[k].Separation, -0.1, 1e-4) || !near(p.Y, 0.95, 1e-3) {
				t.Fatalf("pose %d point %d: %+v local %v", i, k, m.Points[k], p)
			}
			if !near(p.X, want[0], 1e-3) && !near(p.X, want[1], 1e-3) {
				t.Fatalf("pose %d point %d x %v, want one of %v", i, k, p.X, want)
			}
		}
	}
}

func TestCircleBoxRegions(t *testing.T) {
	box := gm(t, Box{Half: Vec2{X: 1, Y: 1}}, Transform{})
	circle := gm(t, Circle{R: 0.5}, Transform{})
	m := collideGeoms(&box, identityXf(), &circle, xfAt(0.3, 1.4, 0))
	if m.Count != 1 || !near(m.Points[0].Separation, -0.1, 1e-5) || !near2(m.Normal, Vec2{Y: 1}, 1e-5) {
		t.Fatalf("face: %+v", m)
	}
	m = collideGeoms(&box, identityXf(), &circle, xfAt(1.3, 1.3, 0))
	d := Vec2{X: 0.3, Y: 0.3}
	if m.Count != 1 || !near(m.Points[0].Separation, d.Len()-0.5, 1e-5) || !near2(m.Normal, d.Normalized(), 1e-5) {
		t.Fatalf("vertex: %+v", m)
	}
	m = collideGeoms(&circle, xfAt(0.3, 1.4, 0), &box, identityXf())
	if m.Count != 1 || !near2(m.Normal, Vec2{Y: -1}, 1e-5) {
		t.Fatalf("flipped: %+v", m)
	}
}

func TestCapsuleBox(t *testing.T) {
	box := gm(t, Box{Half: Vec2{X: 1, Y: 1}}, Transform{})
	cap := gm(t, Capsule{HalfLen: 1, R: 0.25}, Transform{})
	m := collideGeoms(&box, identityXf(), &cap, xfAt(0, 1.2, 0))
	if m.Count != 2 || !near2(m.Normal, Vec2{Y: 1}, 1e-4) {
		t.Fatalf("%+v", m)
	}
	for k := 0; k < 2; k++ {
		if !near(m.Points[k].Separation, -0.05, 1e-4) {
			t.Fatalf("point %d: %+v", k, m.Points[k])
		}
	}
}

func TestFeatureIDsAreStable(t *testing.T) {
	box := gm(t, Box{Half: Vec2{X: 1, Y: 1}}, Transform{})
	ids := func(x float32) []uint16 {
		m := collideGeoms(&box, identityXf(), &box, xfAt(x, 1.95, 0.01))
		out := make([]uint16, m.Count)
		for i := range out {
			out[i] = m.Points[i].ID
		}
		slices.Sort(out)
		return out
	}
	if a, b := ids(0.1), ids(0.1005); !slices.Equal(a, b) || len(a) != 2 {
		t.Fatalf("ids changed: %v %v", a, b)
	}
}

func TestChainGhostsOnJunction(t *testing.T) {
	chain := gm(t, Chain{Pts: []Vec2{{X: -4}, {}, {X: 4}}}, Transform{})
	cap := gm(t, Capsule{HalfLen: 0.5, R: 0.25}, Transform{})
	for x := float32(-1.5); x <= 1.5; x += 0.05 {
		hits := 0
		for i := 0; i < chain.children(); i++ {
			seg, g0, g3 := chain.chain.segment(i)
			var m Manifold
			collideChainSegment(&seg, g0, g3, true, identityXf(), &cap.c, xfAt(x, -0.24, 0), 0.02, &m)
			if m.Count == 0 {
				continue
			}
			hits++
			if m.Normal.Dot(Vec2{Y: -1}) < 0.999 {
				t.Fatalf("x=%v segment %d: false normal %v", x, i, m.Normal)
			}
		}
		if hits == 0 {
			t.Fatalf("x=%v: capsule lost contact with the ground", x)
		}
	}
}

func TestOneSidedIgnoresBack(t *testing.T) {
	chain := gm(t, Chain{Pts: []Vec2{{X: -4}, {}, {X: 4}}}, Transform{})
	circle := gm(t, Circle{R: 0.6}, Transform{})
	for _, oneSided := range []bool{true, false} {
		hits := 0
		for i := 0; i < chain.children(); i++ {
			seg, g0, g3 := chain.chain.segment(i)
			var m Manifold
			collideChainSegment(&seg, g0, g3, oneSided, identityXf(), &circle.c, xfAt(0.5, 0.5, 0), 0.02, &m)
			if m.Count > 0 {
				hits++
				if !near2(m.Normal, Vec2{Y: 1}, 1e-3) {
					t.Fatalf("two-sided normal %v", m.Normal)
				}
			}
		}
		if oneSided && hits != 0 {
			t.Fatal("one-sided chain caught a circle from behind")
		}
		if !oneSided && hits == 0 {
			t.Fatal("two-sided chain missed the circle")
		}
	}
}
