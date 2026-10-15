package world

import "testing"

func TestDistanceJointHoldsLength(t *testing.T) {
	w := newTestWorld(t, nil)
	anchor := must(w.CreateBody(BodyDef{Type: Static, Position: Vec2{Y: -10}, Mask: ^uint32(0)}))
	b, _ := addDynamic(t, w, Vec2{X: 2, Y: -10}, 0, Circle{R: 0.3}, func(d *BodyDef, _ *ShapeDef) { d.AllowSleep = false })
	must(w.CreateJoint(DistanceJoint{A: anchor, B: b}))
	for i := 0; i < 180; i++ {
		w.Step(dt)
		if d := posOf(w, b).Sub(Vec2{Y: -10}).Len(); !near(d, 2, 0.02) {
			t.Fatalf("step %d: length %v", i, d)
		}
	}
}

func TestSoftDistanceConverges(t *testing.T) {
	w := newTestWorld(t, nil)
	anchor := must(w.CreateBody(BodyDef{Type: Static, Position: Vec2{Y: -10}, Mask: ^uint32(0)}))
	b, _ := addDynamic(t, w, Vec2{Y: -8}, 0, Circle{R: 0.5}, nil)
	must(w.CreateJoint(DistanceJoint{A: anchor, B: b, Length: 2, Stiffness: 50, Damping: 2}))
	run(w, 600)
	mass, _, _, _ := w.BodyMass(b)
	want := 2 + mass*9.81/50
	if d := posOf(w, b).Sub(Vec2{Y: -10}).Len(); !near(d, want, 0.05) {
		t.Fatalf("soft length %v, want %v", d, want)
	}
	if v := velOf(w, b); v.Len() > 0.05 {
		t.Fatalf("soft joint still moves with %v", v)
	}
}

func TestRevoluteLimit(t *testing.T) {
	w := newTestWorld(t, nil)
	hinge := must(w.CreateBody(BodyDef{Type: Static, Position: Vec2{Y: -10}, Mask: ^uint32(0)}))
	bar, _ := addDynamic(t, w, Vec2{X: 1, Y: -10}, 0, Box{Half: Vec2{X: 1, Y: 0.1}}, func(d *BodyDef, _ *ShapeDef) { d.AllowSleep = false })
	j := must(w.CreateJoint(RevoluteJoint{A: hinge, B: bar, LocalAnchorB: Vec2{X: -1}, EnableLimit: true, Lower: -0.5, Upper: 0.5}))
	for i := 0; i < 180; i++ {
		w.Step(dt)
		a, _ := w.RevoluteAngle(j)
		if a < -0.55 || a > 0.55 {
			t.Fatalf("step %d: angle %v outside the limit", i, a)
		}
	}
}

func motorSpeed(t *testing.T, torque float32) float32 {
	w := newTestWorld(t, func(c *Config) { c.Gravity = Vec2{} })
	hub := must(w.CreateBody(BodyDef{Type: Static, Position: Vec2{Y: -10}, Mask: ^uint32(0)}))
	wheel, _ := addDynamic(t, w, Vec2{Y: -10}, 0, Circle{R: 0.5}, nil)
	must(w.CreateJoint(RevoluteJoint{A: hub, B: wheel, EnableMotor: true, Speed: 2, MaxTorque: torque}))
	run(w, 60)
	_, av, _ := w.BodyVelocity(wheel)
	return av
}

func TestRevoluteMotor(t *testing.T) {
	if av := motorSpeed(t, 1000); !near(av, 2, 0.02) {
		t.Fatalf("motor reached %v, want 2", av)
	}
	if av := motorSpeed(t, 0.01); av <= 0 || av > 0.2 {
		t.Fatalf("weak motor reached %v", av)
	}
}
