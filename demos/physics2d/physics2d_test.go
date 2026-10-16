package physics2d

import (
	"testing"

	"aego/modules/physics/world"
)

func TestSceneRunsHeadless(t *testing.T) {
	w, err := buildScene()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		w.Step(1.0 / 60)
	}
	if s := w.Stats(); s.Bodies != 666 || s.TouchingPairs == 0 {
		t.Fatalf("stats %+v", s)
	}
	fell := 0
	w.ForEachBody(func(h world.BodyHandle) bool {
		if p, _, _ := w.BodyTransform(h); p.Y > 1 {
			fell++
		}
		return true
	})
	if fell > 0 {
		t.Fatalf("%d bodies fell off the ground", fell)
	}
}

func BenchmarkScene(b *testing.B) {
	w, err := buildScene()
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		w.Step(1.0 / 60)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Step(1.0 / 60)
	}
	var _ world.Stats = w.Stats()
}
