package world

import (
	"os"
	"testing"
	"time"
)

func stacks(tb testing.TB, sleep bool) *World {
	w := newTestWorld(tb, func(c *Config) {
		c.InitialBodies, c.InitialShapes, c.InitialContacts = 4096, 4096, 8192
	})
	groundBox(tb, w, nil)
	for s := 0; s < 100; s++ {
		for k := 0; k < 10; k++ {
			addDynamic(tb, w, Vec2{X: -125 + float32(s)*2.5, Y: -0.5 - float32(k)}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, func(d *BodyDef, _ *ShapeDef) { d.AllowSleep = sleep })
		}
	}
	for i := 0; i < 2000; i++ {
		addStatic(tb, w, Vec2{X: -150 + float32(i%100)*3, Y: 20 + float32(i/100)*3}, 0, Box{Half: Vec2{X: 0.4, Y: 0.4}}, nil)
	}
	return w
}

func TestStepZeroAllocs(t *testing.T) {
	w := stacks(t, false)
	run(w, 600)
	if allocs := testing.AllocsPerRun(100, func() { w.Step(dt) }); allocs != 0 {
		t.Fatalf("step allocates %v times after warmup", allocs)
	}
	s := w.Stats()
	if s.TouchingPairs == 0 || s.Islands == 0 || s.Active != 1000 {
		t.Fatalf("stats %+v", s)
	}
}

func TestStatsCountBroadphase(t *testing.T) {
	w := stacks(t, true)
	w.Step(dt)
	if s := w.Stats(); s.BroadphaseCandidates == 0 || s.Steps != 1 || s.Bodies != 3001 {
		t.Fatalf("stats %+v", s)
	}
}

func avgStep(w *World, n int) time.Duration {
	start := time.Now()
	run(w, n)
	return time.Since(start) / time.Duration(n)
}

func bestStep(w *World, rounds, n int) time.Duration {
	best := time.Duration(1<<63 - 1)
	for i := 0; i < rounds; i++ {
		best = min(best, avgStep(w, n))
	}
	return best
}

// TestStepBudget is a smoke run; times are gated by make bench-physics.
func TestStepBudget(t *testing.T) {
	if os.Getenv("AEGO_PHYSICS_PERF") == "" {
		t.Skip("set AEGO_PHYSICS_PERF=1 to run the step smoke test")
	}
	w := stacks(t, false)
	run(w, 120)
	active := bestStep(w, 3, 240)
	sl := newTestWorld(t, func(c *Config) { c.InitialBodies, c.InitialShapes, c.InitialContacts = 10240, 10240, 16384 })
	addStatic(t, sl, Vec2{Y: 1}, 0, Box{Half: Vec2{X: 2600, Y: 1}}, nil)
	for i := 0; i < 10000; i++ {
		addDynamic(t, sl, Vec2{X: -2500 + float32(i)*0.5, Y: -0.2}, 0, Box{Half: Vec2{X: 0.2, Y: 0.2}}, nil)
	}
	run(sl, 120)
	if s := sl.Stats(); s.Sleeping != 10000 {
		t.Fatalf("not asleep: %+v", s)
	}
	sleeping := bestStep(sl, 3, 200)
	t.Logf("1k active %v, 10k sleeping %v per step", active, sleeping)
}

func BenchmarkStep1kActive(b *testing.B) {
	w := stacks(b, false)
	run(w, 120)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step(dt)
	}
	s := w.Stats()
	b.ReportMetric(float64(s.TouchingPairs), "pairs")
	b.ReportMetric(float64(s.ContactPoints), "points")
}

func BenchmarkRaycast(b *testing.B) {
	w := stacks(b, true)
	run(w, 60)
	f := DefaultQueryFilter()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Raycast(Vec2{X: -120, Y: -5}, Vec2{X: 1, Y: 0.05}, 300, f)
	}
}

func TestSleepingWorldIsCheap(t *testing.T) {
	w := newTestWorld(t, func(c *Config) { c.InitialBodies, c.InitialShapes, c.InitialContacts = 10240, 10240, 16384 })
	addStatic(t, w, Vec2{Y: 1}, 0, Box{Half: Vec2{X: 2600, Y: 1}}, nil)
	for i := 0; i < 10000; i++ {
		addDynamic(t, w, Vec2{X: -2500 + float32(i)*0.5, Y: -0.2}, 0, Box{Half: Vec2{X: 0.2, Y: 0.2}}, nil)
	}
	run(w, 120)
	if s := w.Stats(); s.Sleeping != 10000 || s.Active != 0 {
		t.Fatalf("stats %+v", s)
	}
	if allocs := testing.AllocsPerRun(50, func() { w.Step(dt) }); allocs != 0 {
		t.Fatalf("sleeping step allocates %v times", allocs)
	}
}
