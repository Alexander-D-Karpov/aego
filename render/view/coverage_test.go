package view

import (
	"aego/core/mathx"
	"testing"
)

// Exhaust all 81 combinations of opaque/transparent/no clear, including
// overlap, gaps and a last no-clear view. Compare to the per-pixel model.
func TestComposedCameraCoverage(t *testing.T) {
	areas := []mathx.Rect{{W: 5, H: 5}, {X: 2, W: 4, H: 5}, {Y: 1, W: 6, H: 2}, {W: 6, H: 5}}
	for combination := 0; combination < 81; combination++ {
		var set OutputCoverage
		var alpha [5][6]bool
		flags := combination
		for i, r := range areas {
			op := flags % 3
			flags /= 3
			v := View{Viewport: r, DoClear: op != 0, TargetW: 6, TargetH: 5}
			if op == 2 {
				v.Clear.A = 1
			}
			set.Add(v, int32(i))
			if v.DoClear {
				for y := 0; y < 5; y++ {
					for x := 0; x < 6; x++ {
						if r.Contains(mathx.V2(float32(x)+.5, float32(y)+.5)) {
							alpha[y][x] = v.Clear.A == 1
						}
					}
				}
			}
		}
		opaque := true
		for _, row := range alpha {
			for _, v := range row {
				opaque = opaque && v
			}
		}
		if set.Opaque() != opaque {
			t.Fatalf("clear combination %d: opaque=%v, want %v", combination, !opaque, opaque)
		}
	}
}

func TestCoverageReusesStorageAndIgnoresGapsOutsideCameraRegions(t *testing.T) {
	var set OutputCoverage
	check := func() {
		set.Reset()
		for _, r := range [...]mathx.Rect{{W: 4, H: 4}, {X: 6, W: 4, H: 4}} {
			v := View{TargetW: 10, TargetH: 4, Viewport: r, DoClear: true, Clear: mathx.Color{A: 1}}
			set.Add(v, 0)
			v.DoClear = false
			set.Add(v, 1)
		}
		if !set.Opaque() {
			t.Fatal("unprocessed gap marked transparent")
		}
	}
	if got := testing.AllocsPerRun(25, check); got != 0 {
		t.Fatalf("warmed coverage allocation count %.1f", got)
	}
}

func TestFractionalTransparentClearAffectsAdjacentViewportPixel(t *testing.T) {
	var s OutputCoverage
	v := View{Viewport: mathx.Rect{W: 6, H: 6}, TargetW: 6, TargetH: 6, DoClear: true, Clear: mathx.White}
	s.Add(v, 0)
	v.Viewport = mathx.Rect{X: .75, Y: .25, W: 2.5, H: 5.5}
	v.Clear.A = 0
	s.Add(v, 1)
	// X=3 is covered by ceil(.75+2.5), but not by trunc(.75)+trunc(2.5).
	v.Viewport = mathx.Rect{W: 3, H: 6}
	v.Clear.A = 1
	s.Add(v, 2)
	if s.Opaque() {
		t.Fatal("column 3 is still transparent")
	}
	v.Viewport = mathx.Rect{X: 3, W: 1, H: 6}
	s.Add(v, 3)
	if !s.Opaque() {
		t.Fatal("opaque repair of column 3 was ignored")
	}
}
