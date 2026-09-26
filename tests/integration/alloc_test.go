package integration

import "testing"

func TestZeroAllocationsPerFrame(t *testing.T) {
	e, demo := newEngine(t)
	defer e.Close()
	for i := 0; i < 60; i++ {
		if !e.Step(demo) {
			t.Fatalf("warmup stopped at frame %d: %v", i, e.Err())
		}
	}
	allocs := testing.AllocsPerRun(540, func() {
		e.Step(demo)
	})
	if allocs != 0 {
		t.Fatalf("frame allocates %.1f objects after warmup", allocs)
	}
}
