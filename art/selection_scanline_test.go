package art

import (
	"image"
	"math/rand"
	"testing"
)

func TestLassoScanlineMatchesEvenOddPixelCenters(t *testing.T) {
	rng := rand.New(rand.NewSource(125))
	for trial := 0; trial < 500; trial++ {
		points := make([]image.Point, 3+rng.Intn(15))
		for i := range points {
			points[i] = image.Pt(rng.Intn(32)-8, rng.Intn(32)-8)
		}
		sel, err := LassoSelection(16, 16, points)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				inside := false
				px, py := float64(x)+.5, float64(y)+.5
				j := len(points) - 1
				for i, p := range points {
					q := points[j]
					if (float64(p.Y) > py) != (float64(q.Y) > py) && px < float64(q.X-p.X)*(py-float64(p.Y))/float64(q.Y-p.Y)+float64(p.X) {
						inside = !inside
					}
					j = i
				}
				if sel.Contains(x, y) != inside {
					t.Fatalf("trial %d pixel %d,%d, points=%v: got %v want %v", trial, x, y, points, sel.Contains(x, y), inside)
				}
			}
		}
	}
}

func TestSelectionCachesCountAndBounds(t *testing.T) {
	sel, err := NewSelection(10, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{2, 3}, {7, 6}, {2, 3}, {-1, 0}} {
		sel.set(p.X, p.Y)
	}
	if sel.Count() != 2 || sel.Bounds() != image.Rect(2, 3, 8, 7) {
		t.Fatalf("selection count/bounds: %d %v", sel.Count(), sel.Bounds())
	}
}
