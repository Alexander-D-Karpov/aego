package world

import "testing"

func TestShapeEnabled(t *testing.T) {
	w, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	gd := DefaultBodyDef()
	gd.Type = Static
	g, _ := w.CreateBody(gd)
	if _, err := w.AddShape(g, DefaultShapeDef(Box{Half: Vec2{X: 5, Y: 0.5}})); err != nil {
		t.Fatal(err)
	}
	bd := DefaultBodyDef()
	bd.Position = Vec2{Y: -1}
	b, _ := w.CreateBody(bd)
	s, err := w.AddShape(b, DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}}))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		w.Step(1.0 / 60)
	}
	if w.Stats().TouchingPairs != 1 {
		t.Fatalf("touching %d before disable", w.Stats().TouchingPairs)
	}
	if err := w.SetShapeEnabled(s, false); err != nil {
		t.Fatal(err)
	}
	w.Step(1.0 / 60)
	if countKind(w.Events().Contacts, EndContact) != 1 || w.Stats().TouchingPairs != 0 {
		t.Fatalf("end events %d, touching %d", countKind(w.Events().Contacts, EndContact), w.Stats().TouchingPairs)
	}
	if w.ShapeEnabled(s) {
		t.Fatal("shape still reports enabled")
	}
	if err := w.SetShapeEnabled(s, true); err != nil {
		t.Fatal(err)
	}
	w.Step(1.0 / 60)
	if countKind(w.Events().Contacts, BeginContact) != 1 {
		t.Fatalf("begin events %d after enable", countKind(w.Events().Contacts, BeginContact))
	}
}

func TestDisabledShapeIgnoredByMass(t *testing.T) {
	w, _ := New(DefaultConfig())
	b, _ := w.CreateBody(DefaultBodyDef())
	a, _ := w.AddShape(b, DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}}))
	w.AddShape(b, DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}}))
	full, _, _, _ := w.BodyMass(b)
	w.SetShapeEnabled(a, false)
	half, _, _, _ := w.BodyMass(b)
	if half*2 != full {
		t.Fatalf("mass %v with one shape disabled, %v with both", half, full)
	}
}

func TestForceRecordsOnlyWhenEnabled(t *testing.T) {
	w, _ := New(DefaultConfig())
	b, _ := w.CreateBody(DefaultBodyDef())
	w.AddShape(b, DefaultShapeDef(Circle{R: 0.5}))
	w.ApplyForce(b, Vec2{X: 1})
	w.Step(1.0 / 60)
	if len(w.Forces()) != 0 {
		t.Fatal("forces recorded with debug vectors off")
	}
	w.SetDebugVectors(true)
	w.ApplyImpulse(b, Vec2{Y: -2})
	w.Step(1.0 / 60)
	if f := w.Forces(); len(f) != 1 || !f[0].Impulse || f[0].Vector.Y != -2 {
		t.Fatalf("forces %+v", f)
	}
	w.Step(1.0 / 60)
	if len(w.Forces()) != 0 {
		t.Fatal("forces of the previous step leaked")
	}
}
