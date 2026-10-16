package world

import (
	"math"
	"testing"
)

func TestStackSettles(t *testing.T) {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	var boxes []BodyHandle
	for i := 0; i < 10; i++ {
		b, _ := addDynamic(t, w, Vec2{Y: -0.5 - float32(i)}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, func(d *BodyDef, _ *ShapeDef) { d.AllowSleep = false })
		boxes = append(boxes, b)
	}
	run(w, 300)
	mid := make([]Vec2, len(boxes))
	for i, b := range boxes {
		mid[i] = posOf(w, b)
	}
	run(w, 300)
	for i, b := range boxes {
		p := posOf(w, b)
		if !near2(p, mid[i], 1e-3) {
			t.Fatalf("box %d drifted from %v to %v", i, mid[i], p)
		}
	}
	if top := posOf(w, boxes[9]); !near(top.X, 0, 0.05) || !near(top.Y, -9.5, 0.1) {
		t.Fatalf("stack fell: top at %v", top)
	}
}

func TestPyramidStands(t *testing.T) {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	type item struct {
		b BodyHandle
		p Vec2
	}
	var all []item
	const rows = 15
	for r := 0; r < rows; r++ {
		n := rows - r
		for k := 0; k < n; k++ {
			p := Vec2{X: (float32(k) - float32(n-1)/2) * 0.5, Y: -0.25 - float32(r)*0.5}
			b, _ := addDynamic(t, w, p, 0, Box{Half: Vec2{X: 0.25, Y: 0.25}}, nil)
			all = append(all, item{b, p})
		}
	}
	run(w, 600)
	for i, it := range all {
		if p := posOf(w, it.b); p.Sub(it.p).Len() > 0.2 {
			t.Fatalf("box %d moved from %v to %v", i, it.p, p)
		}
	}
}

func slopeBox(t *testing.T, mu float32) float32 {
	w := newTestWorld(t, nil)
	a := float32(math.Pi / 6)
	slope := addStatic(t, w, Vec2{}, a, Box{Half: Vec2{X: 20, Y: 0.5}}, func(s *ShapeDef) { s.Friction = mu })
	sp, _, _ := w.BodyTransform(slope)
	start := xform{p: sp, q: makeRot(a)}.apply(Vec2{Y: -0.75})
	b, _ := addDynamic(t, w, start, a, Box{Half: Vec2{X: 0.25, Y: 0.25}}, func(_ *BodyDef, s *ShapeDef) { s.Friction = mu })
	run(w, 120)
	return posOf(w, b).Sub(start).Len()
}

func TestSlopeFriction(t *testing.T) {
	if d := slopeBox(t, 0.6); d > 0.05 {
		t.Fatalf("mu=0.6 slid %v on 30 degrees", d)
	}
	if d := slopeBox(t, 0.4); d < 0.5 {
		t.Fatalf("mu=0.4 slid only %v on 30 degrees", d)
	}
}

func bounce(t *testing.T, e float32) float32 {
	w := newTestWorld(t, func(c *Config) { c.Gravity = Vec2{} })
	groundBox(t, w, func(s *ShapeDef) { s.Restitution = e })
	b, _ := addDynamic(t, w, Vec2{Y: -3}, 0, Circle{R: 0.5}, func(d *BodyDef, s *ShapeDef) {
		d.LinearVelocity = Vec2{Y: 5}
		s.Restitution = e
	})
	run(w, 60)
	return velOf(w, b).Y
}

func TestRestitution(t *testing.T) {
	if v := bounce(t, 1); !near(v, -5, 0.05) {
		t.Fatalf("e=1 bounced with %v, want -5", v)
	}
	if v := bounce(t, 0); !near(v, 0, 0.05) {
		t.Fatalf("e=0 bounced with %v, want 0", v)
	}
}

func TestRestingBallDoesNotJitter(t *testing.T) {
	w := newTestWorld(t, nil)
	groundBox(t, w, func(s *ShapeDef) { s.Restitution = 0.9 })
	b, _ := addDynamic(t, w, Vec2{Y: -0.5}, 0, Circle{R: 0.5}, func(d *BodyDef, s *ShapeDef) {
		d.AllowSleep = false
		s.Restitution = 0.9
	})
	run(w, 180)
	if v := velOf(w, b); v.Len() > 0.05 {
		t.Fatalf("resting ball moves with %v", v)
	}
	if p := posOf(w, b); !near(p.Y, -0.5, 0.02) {
		t.Fatalf("resting ball at %v", p)
	}
}

func TestBulletDoesNotTunnel(t *testing.T) {
	w := newTestWorld(t, func(c *Config) { c.Gravity = Vec2{} })
	addStatic(t, w, Vec2{X: 10}, 0, Box{Half: Vec2{X: 0.05, Y: 5}}, nil)
	b, _ := addDynamic(t, w, Vec2{}, 0, Circle{R: 0.1}, func(d *BodyDef, _ *ShapeDef) {
		d.Bullet = true
		d.LinearVelocity = Vec2{X: 200}
	})
	for i := 0; i < 60; i++ {
		w.Step(dt)
		if p := posOf(w, b); p.X > 9.95 {
			t.Fatalf("bullet passed the wall at step %d: %v", i, p)
		}
	}
}

func TestMaxTranslationClamps(t *testing.T) {
	w := newTestWorld(t, func(c *Config) { c.Gravity = Vec2{} })
	b, _ := addDynamic(t, w, Vec2{}, 0, Circle{R: 0.5}, func(d *BodyDef, _ *ShapeDef) { d.LinearVelocity = Vec2{X: 1000} })
	w.Step(dt)
	if p := posOf(w, b); p.X > w.cfg.MaxTranslationPerStep+1e-4 {
		t.Fatalf("body moved %v in one step", p.X)
	}
}

func TestNaNImpulseIsRejected(t *testing.T) {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	b, _ := addDynamic(t, w, Vec2{Y: -2}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, nil)
	run(w, 10)
	before := w.Snapshot()
	nan := float32(math.NaN())
	if err := w.ApplyImpulse(b, Vec2{X: nan}); err == nil {
		t.Fatal("NaN impulse accepted")
	}
	if err := w.ApplyForceAtPoint(b, Vec2{X: 1}, Vec2{Y: nan}); err == nil {
		t.Fatal("NaN point accepted")
	}
	if err := w.SetBodyTransform(b, Vec2{X: float32(math.Inf(1))}, 0); err == nil {
		t.Fatal("infinite transform accepted")
	}
	if !before.Equal(w.Snapshot()) {
		t.Fatal("rejected input changed the world")
	}
}

func columnTop(t *testing.T, n int, mod func(*Config)) Vec2 {
	w := newTestWorld(t, mod)
	groundBox(t, w, nil)
	var top BodyHandle
	for i := 0; i < n; i++ {
		top, _ = addDynamic(t, w, Vec2{Y: -0.5 - float32(i)}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, func(d *BodyDef, _ *ShapeDef) { d.AllowSleep = false })
	}
	run(w, 600)
	return posOf(w, top)
}

func TestTallColumnStands(t *testing.T) {
	if p := columnTop(t, 12, nil); !near(p.X, 0, 0.05) || !near(p.Y, -11.5, 0.1) {
		t.Fatalf("column leans: top at %v", p)
	}
}

func TestTallColumnStandsStiff(t *testing.T) {
	stiff := func(c *Config) {
		c.ContactHertz, c.Substeps, c.VelocityIterations, c.RelaxIterations = 60, 8, 1, 1
	}
	if p := columnTop(t, 20, stiff); !near(p.X, 0, 0.05) || !near(p.Y, -19.5, 0.1) {
		t.Fatalf("column leans: top at %v", p)
	}
}

func TestConfigIterationsZeroMeansDefault(t *testing.T) {
	w := must(New(Config{}))
	if c := w.Config(); c.VelocityIterations != 1 || c.RelaxIterations != 1 {
		t.Fatalf("zero iterations did not take the defaults: %+v", c)
	}
	for _, c := range []Config{{RelaxIterations: -1}, {VelocityIterations: -1}, {RelaxIterations: 65}} {
		if _, err := New(c); err == nil {
			t.Errorf("config %+v must be rejected", c)
		}
	}
}
