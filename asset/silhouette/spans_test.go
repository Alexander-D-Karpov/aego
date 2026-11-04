package silhouette

import (
	"image/png"
	"os"
	"testing"
)

func TestRunnerEveryOpaquePixel(t *testing.T) {
	f, e := os.Open("../../examples/platformer/assets/surfaces/runner.png")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	im, e := png.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	b := im.Bounds()
	pix := make([]byte, 4*b.Dx()*b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a := im.At(x, y).RGBA()
			pix[(y*b.Dx()+x)*4+3] = byte(a >> 8)
		}
	}
	if b.Dx() != 128 || b.Dy() != 16 {
		t.Fatal(b)
	}
	for frame := 0; frame < 8; frame++ {
		s, e := Trace(pix, 128, 16, Rect{16 * frame, 0, 16, 16}, 16, 16, 0, 0)
		if e != nil {
			t.Fatal(e)
		}
		for y := 0; y < 16; y++ {
			for x := 0; x < 16; x++ {
				count := 0
				for _, r := range s.Parts {
					if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
						count++
					}
				}
				want := 0
				if pix[(y*128+16*frame+x)*4+3] >= 128 {
					want = 1
				}
				if count != want {
					t.Fatalf("frame %d pixel %d,%d: %d != %d", frame, x, y, count, want)
				}
			}
		}
	}
}
func TestTrimAndBudget(t *testing.T) {
	p := make([]byte, 5*4)
	p[2*4+3] = 255
	s, e := Trace(p, 5, 1, Rect{2, 0, 1, 1}, 7, 9, 3, 4)
	if e != nil || len(s.Parts) != 1 || s.Parts[0] != (Rect{3, 4, 1, 1}) {
		t.Fatal(s, e)
	}
	p = make([]byte, 64*64*4)
	for i := 0; i < 4096; i++ {
		if (i+i/64)%2 == 0 {
			p[4*i+3] = 255
		}
	}
	if _, e = Trace(p, 64, 64, Rect{0, 0, 64, 64}, 64, 64, 0, 0); e == nil {
		t.Fatal("oversized checkerboard accepted")
	}
}
