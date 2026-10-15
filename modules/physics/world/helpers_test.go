package world

import (
	"math"
	"testing"
)

const dt = float32(1.0 / 60)

func newTestWorld(t testing.TB, mod func(*Config)) *World {
	t.Helper()
	cfg := DefaultConfig()
	if mod != nil {
		mod(&cfg)
	}
	w, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func noErr(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func addStatic(t testing.TB, w *World, p Vec2, angle float32, g Geometry, mod func(*ShapeDef)) BodyHandle {
	t.Helper()
	d := DefaultBodyDef()
	d.Type, d.Position, d.Angle = Static, p, angle
	b, err := w.CreateBody(d)
	noErr(t, err)
	sd := DefaultShapeDef(g)
	if mod != nil {
		mod(&sd)
	}
	_, err = w.AddShape(b, sd)
	noErr(t, err)
	return b
}

func addDynamic(t testing.TB, w *World, p Vec2, angle float32, g Geometry, mod func(*BodyDef, *ShapeDef)) (BodyHandle, ShapeHandle) {
	t.Helper()
	d := DefaultBodyDef()
	d.Position, d.Angle = p, angle
	sd := DefaultShapeDef(g)
	if mod != nil {
		mod(&d, &sd)
	}
	b, err := w.CreateBody(d)
	noErr(t, err)
	s, err := w.AddShape(b, sd)
	noErr(t, err)
	return b, s
}

func groundBox(t testing.TB, w *World, mod func(*ShapeDef)) BodyHandle {
	return addStatic(t, w, Vec2{Y: 1}, 0, Box{Half: Vec2{X: 200, Y: 1}}, mod)
}

func run(w *World, n int) {
	for i := 0; i < n; i++ {
		w.Step(dt)
	}
}

func near(a, b, eps float32) bool {
	return math.Abs(float64(a-b)) <= float64(eps)
}

func near2(a, b Vec2, eps float32) bool {
	return near(a.X, b.X, eps) && near(a.Y, b.Y, eps)
}

func posOf(w *World, b BodyHandle) Vec2 {
	p, _, _ := w.BodyTransform(b)
	return p
}

func velOf(w *World, b BodyHandle) Vec2 {
	v, _, _ := w.BodyVelocity(b)
	return v
}

func gm(t testing.TB, g Geometry, local Transform) geom {
	t.Helper()
	out, err := buildGeom(g, local, 0.005)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func xfAt(x, y, a float32) xform {
	return xform{p: Vec2{X: x, Y: y}, q: makeRot(a)}
}
