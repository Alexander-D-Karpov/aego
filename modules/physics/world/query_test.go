package world

import (
	"math"
	"testing"
)

func castOne(t *testing.T, g Geometry, at Vec2, from, dir Vec2) RayHit {
	w := newTestWorld(t, nil)
	addStatic(t, w, at, 0, g, nil)
	hit, ok := w.Raycast(from, dir, 20, DefaultQueryFilter())
	if !ok {
		t.Fatalf("%T missed", g)
	}
	return hit
}

func TestRaycastShapes(t *testing.T) {
	var tri Polygon
	tri.N = 3
	tri.Pts[0], tri.Pts[1], tri.Pts[2] = Vec2{X: -1, Y: 1}, Vec2{X: 1, Y: 1}, Vec2{Y: -1}
	cases := []struct {
		g         Geometry
		from, dir Vec2
		dist      float32
		normal    Vec2
		checkNorm bool
	}{
		{Circle{R: 1}, Vec2{}, Vec2{X: 1}, 4, Vec2{X: -1}, true},
		{Box{Half: Vec2{X: 1, Y: 1}}, Vec2{}, Vec2{X: 1}, 4, Vec2{X: -1}, true},
		{Capsule{HalfLen: 1, R: 0.5}, Vec2{}, Vec2{X: 1}, 3.5, Vec2{X: -1}, true},
		{Capsule{HalfLen: 1, R: 0.5}, Vec2{X: 5, Y: -5}, Vec2{Y: 1}, 4.5, Vec2{Y: -1}, true},
		{tri, Vec2{X: 5, Y: -5}, Vec2{Y: 1}, 4, Vec2{}, false},
		{Chain{Pts: []Vec2{{Y: -2}, {Y: 2}}}, Vec2{}, Vec2{X: 1}, 5, Vec2{X: -1}, true},
	}
	for i, c := range cases {
		hit := castOne(t, c.g, Vec2{X: 5}, c.from, c.dir)
		if !near(hit.Distance, c.dist, 1e-4) || (c.checkNorm && !near2(hit.Normal, c.normal, 1e-4)) {
			t.Fatalf("case %d %T: %+v", i, c.g, hit)
		}
	}
}

func rayRow(t *testing.T) (*World, []BodyHandle) {
	w := newTestWorld(t, nil)
	var bs []BodyHandle
	for i, x := range []float32{3, 6, 9} {
		trigger := i == 0
		bs = append(bs, addStatic(t, w, Vec2{X: x}, 0, Circle{R: 0.5}, func(s *ShapeDef) { s.IsTrigger = trigger }))
	}
	return w, bs
}

func TestRaycastAllAndFilters(t *testing.T) {
	w, bs := rayRow(t)
	f := DefaultQueryFilter()
	f.IncludeTriggers = true
	var out [5]RayHit
	if n := w.RaycastAll(Vec2{}, Vec2{X: 1}, 20, f, out[:2]); n != 2 || out[0].Distance != 2.5 || out[1].Distance != 5.5 {
		t.Fatalf("truncated: %d %+v", n, out[:2])
	}
	if n := w.RaycastAll(Vec2{}, Vec2{X: 1}, 20, f, out[:]); n != 3 || out[2].Distance != 8.5 {
		t.Fatalf("all: %d %+v", n, out[:n])
	}
	if h, _ := w.Raycast(Vec2{}, Vec2{X: 1}, 20, DefaultQueryFilter()); h.Body != bs[1] {
		t.Fatalf("triggers were not skipped: %+v", h)
	}
	f.ExcludeBody = bs[0]
	if h, _ := w.Raycast(Vec2{}, Vec2{X: 1}, 20, f); h.Body != bs[1] {
		t.Fatalf("exclude failed: %+v", h)
	}
}

func TestOverlaps(t *testing.T) {
	w := newTestWorld(t, nil)
	in := addStatic(t, w, Vec2{X: 0.6, Y: 0.6}, 0, Circle{R: 0.1}, nil)
	addStatic(t, w, Vec2{X: 0.6, Y: -0.6}, 0, Circle{R: 0.1}, nil)
	f := DefaultQueryFilter()
	var out [4]ShapeHandle
	n := w.OverlapBox(Vec2{}, Vec2{X: 1, Y: 0.05}, math.Pi/4, f, out[:])
	if n != 1 || out[0] != w.BodyShapes(in)[0] {
		t.Fatalf("rotated box: %d %v", n, out[:n])
	}
	if n := w.OverlapCircle(Vec2{X: 0.6}, 0.55, f, out[:]); n != 2 {
		t.Fatalf("circle overlap: %d", n)
	}
	if n := w.OverlapPoint(Vec2{X: 0.62, Y: 0.6}, f, out[:]); n != 1 {
		t.Fatalf("point overlap: %d", n)
	}
	if s, ok := w.ShapeAt(Vec2{X: 0.6, Y: 0.6}, f); !ok || s != w.BodyShapes(in)[0] {
		t.Fatal("ShapeAt missed")
	}
}

func TestQueriesDoNotAllocate(t *testing.T) {
	w, _ := rayRow(t)
	f := DefaultQueryFilter()
	var hits [4]RayHit
	var shapes [4]ShapeHandle
	allocs := testing.AllocsPerRun(200, func() {
		w.Raycast(Vec2{}, Vec2{X: 1}, 20, f)
		w.RaycastAll(Vec2{}, Vec2{X: 1}, 20, f, hits[:])
		w.OverlapBox(Vec2{X: 6}, Vec2{X: 1, Y: 1}, 0.3, f, shapes[:])
		w.OverlapPoint(Vec2{X: 6}, f, shapes[:])
	})
	if allocs != 0 {
		t.Fatalf("queries allocate %v times", allocs)
	}
}
