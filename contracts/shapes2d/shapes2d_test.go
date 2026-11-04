package shapes2d

import (
	"errors"
	"testing"

	"aego/core/mathx"
)

func TestBufferOwnsPointsAndOffsetsSurviveGrowth(t *testing.T) {
	source := []mathx.Vec2{{}, {X: 1}, {Y: 1}}
	b := Buffer{}
	if err := b.Append(Contour{Key: "first", Closed: true}, source); err != nil {
		t.Fatal(err)
	}
	first := b.Contours[0]
	source[1].X = 99
	for i := 0; i < 20; i++ {
		if err := b.Append(Contour{Closed: true}, source); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.Vertices(first); len(got) != 3 || got[1].X != 1 {
		t.Fatalf("first copied contour changed after growth: %v", got)
	}
	capacity := cap(b.Points)
	b.Reset()
	if len(b.Points) != 0 || len(b.Contours) != 0 || cap(b.Points) != capacity {
		t.Fatal("reset did not preserve reusable storage")
	}
	if got := b.Vertices(first); got != nil {
		t.Fatal("stale offsets outside reset buffer remained readable")
	}
}

func TestBufferBudgetsRejectWithoutAppending(t *testing.T) {
	b := Buffer{Limits: Limits{Contours: 2, Points: 5}}
	p := []mathx.Vec2{{}, {X: 1}, {Y: 1}}
	if err := b.Append(Contour{Closed: true}, p); err != nil {
		t.Fatal(err)
	}
	if err := b.Append(Contour{Closed: true}, p); !errors.Is(err, ErrBudget) {
		t.Fatalf("point budget: %v", err)
	}
	if len(b.Contours) != 1 || len(b.Points) != 3 {
		t.Fatal("failed append changed output")
	}
	if err := b.Append(Contour{}, p[:2]); err != nil {
		t.Fatal(err)
	}
	if err := b.Append(Contour{}, p[:2]); !errors.Is(err, ErrBudget) {
		t.Fatalf("contour budget: %v", err)
	}
	b.Reset()
	if err := b.Append(Contour{}, make([]mathx.Vec2, MaxContourPoints+1)); !errors.Is(err, ErrBudget) {
		t.Fatalf("per-contour budget: %v", err)
	}
	if err := (*Buffer)(nil).Append(Contour{}, p); !errors.Is(err, ErrBuffer) {
		t.Fatalf("nil buffer: %v", err)
	}
}
