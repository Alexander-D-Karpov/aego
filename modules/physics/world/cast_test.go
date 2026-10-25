package world

import "testing"

func castWorld(t *testing.T) *World {
	t.Helper()
	w, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func addBody(t *testing.T, w *World, typ BodyType, p Vec2, shapes ...ShapeDef) BodyHandle {
	t.Helper()
	d := DefaultBodyDef()
	d.Type, d.Position, d.GravityScale = typ, p, 0
	bh, err := w.CreateBody(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shapes {
		if _, err := w.AddShape(bh, s); err != nil {
			t.Fatal(err)
		}
	}
	return bh
}

func floor(t *testing.T, w *World) {
	addBody(t, w, Static, Vec2{Y: 5}, DefaultShapeDef(Box{Half: Vec2{X: 5, Y: 0.5}}))
	w.Step(1.0 / 60)
}

func TestCastCircleHitsFloor(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	h, ok := w.CastCircle(Vec2{}, 0.5, Vec2{Y: 10}, DefaultQueryFilter())
	if !ok || h.Initial || !near(h.Fraction, 0.4, 0.005) {
		t.Fatalf("hit %+v ok=%v", h, ok)
	}
	if !near(h.Normal.Y, -1, 1e-3) || !near(h.Point.Y, 4.5, 0.01) {
		t.Fatalf("normal %v point %v", h.Normal, h.Point)
	}
}

func TestCastBoxAndCapsule(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	if h, ok := w.CastBox(Vec2{}, Vec2{X: 0.5, Y: 0.5}, 0, Vec2{Y: 10}, DefaultQueryFilter()); !ok || !near(h.Fraction, 0.4, 0.005) {
		t.Fatalf("box %+v", h)
	}
	if h, ok := w.CastCapsule(Vec2{X: -0.5}, Vec2{X: 0.5}, 0.25, Vec2{Y: 10}, DefaultQueryFilter()); !ok || !near(h.Fraction, 0.425, 0.005) {
		t.Fatalf("capsule %+v", h)
	}
}

func TestCastSlidesAlongSurface(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	if h, ok := w.CastBox(Vec2{Y: 3.99}, Vec2{X: 0.5, Y: 0.5}, 0, Vec2{X: 10}, DefaultQueryFilter()); ok {
		t.Fatalf("parallel move hit %+v", h)
	}
	if h, ok := w.CastCircle(Vec2{Y: 3}, 0.5, Vec2{Y: -5}, DefaultQueryFilter()); ok {
		t.Fatalf("moving away hit %+v", h)
	}
}

func TestCastStartsInside(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	h, ok := w.CastCircle(Vec2{Y: 5}, 0.5, Vec2{Y: 10}, DefaultQueryFilter())
	if !ok || !h.Initial || h.Fraction != 0 || h.Depth <= 0 {
		t.Fatalf("initial overlap %+v ok=%v", h, ok)
	}
}

func TestCastBodySkipsItsOwnShapes(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	box := DefaultShapeDef(Box{Half: Vec2{X: 0.25, Y: 0.25}})
	box.Local = Transform{Position: Vec2{Y: -0.5}}
	b := addBody(t, w, Kinematic, Vec2{}, DefaultShapeDef(Capsule{HalfLen: 0.5, R: 0.25}), box)
	h, ok := w.CastBody(b, Vec2{}, Vec2{Y: 10}, DefaultQueryFilter())
	if !ok || h.Body == b || !near(h.Fraction, 0.425, 0.005) {
		t.Fatalf("body cast %+v", h)
	}
}

func TestPenetration(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	b := addBody(t, w, Kinematic, Vec2{Y: 4.2}, DefaultShapeDef(Circle{R: 0.5}))
	var out [4]CastHit
	n := w.Penetration(b, DefaultQueryFilter(), out[:])
	if n != 1 || !near(out[0].Depth, 0.2, 1e-3) || !near(out[0].Normal.Y, -1, 1e-3) {
		t.Fatalf("penetration %d %+v", n, out[0])
	}
}

func TestCastSkipDynamic(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	addBody(t, w, Dynamic, Vec2{Y: 2}, DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}}))
	w.Step(1.0 / 60)
	f := DefaultQueryFilter()
	if h, ok := w.CastCircle(Vec2{}, 0.5, Vec2{Y: 10}, f); !ok || h.Fraction > 0.2 {
		t.Fatalf("dynamic box must block: %+v", h)
	}
	f.SkipDynamic = true
	if h, ok := w.CastCircle(Vec2{}, 0.5, Vec2{Y: 10}, f); !ok || !near(h.Fraction, 0.4, 0.005) {
		t.Fatalf("SkipDynamic must reach the floor: %+v", h)
	}
}

func TestCastAllocs(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	b := addBody(t, w, Kinematic, Vec2{}, DefaultShapeDef(Capsule{HalfLen: 0.5, R: 0.25}))
	var out [4]CastHit
	f := DefaultQueryFilter()
	allocs := testing.AllocsPerRun(200, func() {
		w.CastBody(b, Vec2{}, Vec2{Y: 10}, f)
		w.CastCircle(Vec2{}, 0.5, Vec2{X: 3, Y: 10}, f)
		w.Penetration(b, f, out[:])
	})
	if allocs != 0 {
		t.Fatalf("%v allocations per cast", allocs)
	}
}

func TestCastOneSidedChain(t *testing.T) {
	w := castWorld(t)
	d := DefaultShapeDef(Chain{Pts: []Vec2{{X: -5, Y: 5}, {X: 5, Y: 5}}})
	d.OneSided = true
	addBody(t, w, Static, Vec2{}, d)
	w.Step(1.0 / 60)
	if h, ok := w.CastCircle(Vec2{}, 0.5, Vec2{Y: 10}, DefaultQueryFilter()); !ok || !near(h.Fraction, 0.45, 0.005) {
		t.Fatalf("front side must block: %+v ok=%v", h, ok)
	}
	if h, ok := w.CastCircle(Vec2{Y: 8}, 0.5, Vec2{Y: -10}, DefaultQueryFilter()); ok {
		t.Fatalf("back side must pass: %+v", h)
	}
}

func TestPenetrationBoxCoresOverlap(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	b := addBody(t, w, Kinematic, Vec2{Y: 4.2}, DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}}))
	var out [4]CastHit
	n := w.Penetration(b, DefaultQueryFilter(), out[:])
	if n != 1 || !near(out[0].Depth, 0.2, 1e-3) || !near(out[0].Normal.Y, -1, 1e-3) || !near(out[0].Point.Y, 4.5, 1e-3) {
		t.Fatalf("penetration %d %+v", n, out[0])
	}
	h, ok := w.CastBox(Vec2{Y: 4.2}, Vec2{X: 0.5, Y: 0.5}, 0, Vec2{X: 1}, DefaultQueryFilter())
	if !ok || !h.Initial || !near(h.Depth, 0.2, 1e-3) || !near(h.Normal.Y, -1, 1e-3) {
		t.Fatalf("initial box cast %+v ok=%v", h, ok)
	}
}

func TestCastFromTouchingContact(t *testing.T) {
	w := castWorld(t)
	floor(t, w)
	b := addBody(t, w, Kinematic, Vec2{Y: 4}, DefaultShapeDef(Box{Half: Vec2{X: 0.25, Y: 0.5}}))
	if h, ok := w.CastBody(b, Vec2{}, Vec2{Y: 0.1}, DefaultQueryFilter()); !ok || h.Fraction != 0 || !near(h.Normal.Y, -1, 1e-3) {
		t.Fatalf("cast into the floor it touches %+v ok=%v", h, ok)
	}
	for _, d := range []Vec2{{X: 1}, {X: -1}, {Y: -1}} {
		if h, ok := w.CastBody(b, Vec2{}, d, DefaultQueryFilter()); ok {
			t.Fatalf("cast %v along or away from the floor hit %+v", d, h)
		}
	}
}
