package slice

import (
	"fmt"
	"testing"

	"aego/asset/pipeline"
)

func TestGrid(t *testing.T) {
	got := Grid(40, 20, [2]int{8, 8}, [2]int{2, 1}, [2]int{2, 2}, 0)
	want := []Rect{{2, 1, 8, 8}, {12, 1, 8, 8}, {22, 1, 8, 8}, {32, 1, 8, 8}, {2, 11, 8, 8}, {12, 11, 8, 8}, {22, 11, 8, 8}, {32, 11, 8, 8}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("grid %v", got)
	}
	if got := Grid(40, 20, [2]int{8, 8}, [2]int{2, 1}, [2]int{2, 2}, 5); len(got) != 5 || got[4] != (Rect{2, 11, 8, 8}) {
		t.Fatalf("count 5: %v", got)
	}
	if Grid(40, 20, [2]int{0, 8}, [2]int{}, [2]int{}, 0) != nil {
		t.Fatal("a zero cell gives no cells")
	}
	if Grid(40, 20, [2]int{8, 8}, [2]int{}, [2]int{-8, 0}, 0) != nil {
		t.Fatal("a non-advancing grid must not hang the Inspector")
	}
}

func TestAlpha(t *testing.T) {
	const w, h = 12, 8
	px := make([]byte, w*h*4)
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				px[(y*w+x)*4+3] = 255
			}
		}
	}
	fill(7, 0, 9, 2)
	fill(0, 1, 2, 3)
	fill(3, 3, 3, 3)
	fill(1, 6, 4, 7)
	px[(5*w+11)*4+3] = 10
	got := Alpha(px, w, h, 16)
	want := []Rect{{0, 1, 4, 3}, {7, 0, 3, 3}, {1, 6, 4, 2}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("alpha %v, want %v (diagonal pixels join, rows top to bottom, then left to right, faint pixels ignored)", got, want)
	}
}

func TestReslicePreservesRenamedWorkingFrameAndPivot(t *testing.T) {
	cells := []Rect{{0, 0, 16, 16}, {16, 0, 16, 16}}
	working := Plan("hero", nil, cells)
	working[1].Name = "jump"
	working[1].HasPivot = true
	working[1].Pivot = [2]float32{.25, .75}
	again := Plan("hero", working, []Rect{cells[1], {32, 0, 16, 16}, cells[0]})
	if again[0] != working[1] || again[2] != working[0] || again[1].ID.IsZero() || again[1].ID == working[1].ID || again[1].Name == again[0].Name || again[1].Name == again[2].Name {
		t.Fatalf("reordered/expanded grid lost an authored frame: %+v", again)
	}
}

func TestPlanKeepsIDsByName(t *testing.T) {
	first := Plan("hero", nil, Grid(64, 16, [2]int{16, 16}, [2]int{}, [2]int{}, 0))
	again := Plan("hero", first, Grid(64, 16, [2]int{16, 16}, [2]int{}, [2]int{}, 0))
	for i := range first {
		if first[i].ID != again[i].ID || again[i].Name != fmt.Sprintf("hero_%03d", i) {
			t.Fatalf("cell %d: %v → %v", i, first[i], again[i])
		}
	}
	more := Plan("hero", first, Grid(64, 32, [2]int{16, 16}, [2]int{}, [2]int{}, 0))
	if len(more) != 8 || more[3].ID != first[3].ID || more[4].ID == (pipeline.SubAsset{}).ID {
		t.Fatal("a larger slice keeps the old ids and adds new ones")
	}
}
