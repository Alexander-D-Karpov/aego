package debug

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu/null"
)

func TestSpacesUploadContiguously(t *testing.T) {
	d, err := New(null.New(), 256)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Line(mathx.Vec2{}, mathx.Vec2{X: 1}, mathx.Color{A: 1})
	d.Space(Overlay)
	d.AABB3(mathx.AABB3{Max: mathx.Vec3{X: 1, Y: 1, Z: 1}}, mathx.Color{A: 1})
	d.Space(Screen)
	d.Cross(mathx.Vec2{}, 4, mathx.Color{A: 1})
	w, o, s := d.Upload()
	if w != (Range{0, 2}) || o != (Range{2, 24}) || s != (Range{26, 4}) {
		t.Fatalf("ranges = %v %v %v", w, o, s)
	}
}

func TestBudgetIsShared(t *testing.T) {
	d, _ := New(null.New(), 8)
	defer d.Close()
	d.Grid3(mathx.Vec3{}, 1, 5, mathx.Color{A: 1})
	w, o, s := d.Counts()
	if w+o+s > 8 || d.Dropped() == 0 {
		t.Fatalf("counts=%d,%d,%d dropped=%d", w, o, s, d.Dropped())
	}
	if n := testing.AllocsPerRun(100, func() {
		d.Reset()
		d.Line3(mathx.Vec3{}, mathx.Vec3{X: 1}, mathx.Color{A: 1})
	}); n != 0 {
		t.Fatalf("debug lines allocate: %.1f", n)
	}
}
