package texture

import (
	"aego/core/mathx"
	"testing"
)

func TestTrimRegionMirrorsOriginalCanvas(t *testing.T) {
	r := Region{UVMax: mathx.Vec2{X: 1, Y: 1}, W: 3, H: 2, SourceW: 8, SourceH: 6, TrimX: 2, TrimY: 1}
	if r.Size() != (mathx.Vec2{X: 8, Y: 6}) || !r.Trimmed() {
		t.Fatal("original size lost")
	}
	f := r.FlippedX().FlippedY()
	if f.TrimX != 3 || f.TrimY != 3 || f.FlippedY().FlippedX() != r {
		t.Fatalf("flip trim %+v", f)
	}
	want := [4]float32{.625, .5, .25, 1.0 / 6}
	if local := f.LocalRect(); local != want {
		for i := range local {
			if mathx.Abs(local[i]-want[i]) > 0.000001 {
				t.Fatalf("original local %+v", local)
			}
		}
	}
	sub := r.Sub(3, 0, 4, 4)
	if sub.W != 2 || sub.H != 2 || sub.SourceW != 4 || sub.SourceH != 4 || sub.TrimX != 0 || sub.TrimY != 1 {
		t.Fatalf("source canvas sub %+v", sub)
	}
	if !r.Sub(6, 0, 2, 2).IsZero() {
		t.Fatal("transparent source crop reads another atlas region")
	}
}
