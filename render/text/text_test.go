package text

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/texture"
)

func testFont() *BitmapFont {
	f := &BitmapFont{LineHeight: 13, Base: 11, Space: 7}
	whole := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 128, 128)
	for c := 32; c < 127; c++ {
		f.Glyphs[c] = Glyph{
			Region:  whole.Sub(0, 0, 7, 13),
			Advance: 7,
			Valid:   true,
		}
	}
	return f
}

func TestMeasureMatchesLayout(t *testing.T) {
	f := testFont()
	const s = "hello\nworld!"
	want := f.Measure(s)
	draws := f.Layout(s, mathx.Vec2{}, 1, mathx.Color{R: 1, G: 1, B: 1, A: 1}, 0, 0, nil)
	var maxX, maxY float32
	for i := range draws {
		d := &draws[i]
		size := d.WorldSize(1)
		maxX = max(maxX, d.Position.X+size.X)
		maxY = max(maxY, d.Position.Y+size.Y)
	}
	if mathx.Abs(maxX-want.X) > 0.001 {
		t.Fatalf("width: measured %f, laid out %f", want.X, maxX)
	}
	if mathx.Abs(maxY-want.Y) > 0.001 {
		t.Fatalf("height: measured %f, laid out %f", want.Y, maxY)
	}
}

func TestLayoutSkipsSpaces(t *testing.T) {
	f := testFont()
	draws := f.Layout("a b", mathx.Vec2{}, 1, mathx.Color{}, 0, 0, nil)
	if len(draws) != 2 {
		t.Fatalf("expected 2 glyph sprites, got %d", len(draws))
	}
	if draws[1].Position.X != 14 {
		t.Fatalf("second glyph at %f", draws[1].Position.X)
	}
}

func TestLayoutScale(t *testing.T) {
	f := testFont()
	draws := f.Layout("ab", mathx.Vec2{X: 10, Y: 20}, 2, mathx.Color{}, 0, 0, nil)
	if draws[1].Position.X != 24 {
		t.Fatalf("scaled advance wrong: %f", draws[1].Position.X)
	}
	if draws[0].Size.X != 14 {
		t.Fatalf("scaled glyph size wrong: %f", draws[0].Size.X)
	}
}

func TestMeasureEmpty(t *testing.T) {
	f := testFont()
	if got := f.Measure(""); got.X != 0 || got.Y != f.LineHeight {
		t.Fatalf("empty measure = %v", got)
	}
}

var _ = sprite.Draw{}
