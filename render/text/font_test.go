package text

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/texture"
)

func unicodeFont() *Font {
	f := NewFont(14, 2, 18, 14)
	whole := texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 256, 256)
	for _, r := range "AVabcПрив:3 ?" {
		g := FontGlyph{Advance: 8}
		if r != ' ' {
			g.Region = whole.Sub(0, 0, 16, 28)
			g.Size = mathx.Vec2{X: 8, Y: 14}
		}
		f.SetGlyph(r, g)
	}
	f.SetKern('A', 'V', -2)
	return f
}

func TestUTF8MeasureMatchesLayout(t *testing.T) {
	f := unicodeFont()
	const s = "Привет AV"
	want := f.Measure(s)
	draws := f.Layout(s, mathx.Vec2{}, 1, mathx.Color{A: 1}, 0, 0, nil)
	if len(draws) != 8 {
		t.Fatalf("glyph sprites = %d", len(draws))
	}
	last := draws[len(draws)-1]
	if got := last.Position.X + f.Advance('V'); mathx.Abs(got-want.X) > 1e-3 {
		t.Fatalf("layout end %v, measure %v", got, want.X)
	}
}

func TestKerningApplied(t *testing.T) {
	f := unicodeFont()
	if w := f.Measure("AV").X; w != 14 {
		t.Fatalf("kerned width = %v", w)
	}
	if w := f.Measure("VA").X; w != 16 {
		t.Fatalf("unkerned width = %v", w)
	}
}

func TestIndexAtRunes(t *testing.T) {
	f := unicodeFont()
	rs := []rune("Прив")
	if i := f.IndexAtRunes(rs, 3); i != 0 {
		t.Fatalf("index = %d", i)
	}
	if i := f.IndexAtRunes(rs, 13); i != 2 {
		t.Fatalf("index = %d", i)
	}
	if i := f.IndexAtRunes(rs, 1000); i != 4 {
		t.Fatalf("index = %d", i)
	}
}

func TestFallbackGlyph(t *testing.T) {
	f := unicodeFont()
	draws := f.Layout("Ж", mathx.Vec2{}, 1, mathx.Color{A: 1}, 0, 0, nil)
	if len(draws) != 1 {
		t.Fatal("missing glyph must render the fallback")
	}
}

func TestFontSetClosest(t *testing.T) {
	var set FontSet
	a := NewFont(12, 1, 15, 12)
	b := NewFont(14, 1, 17, 14)
	c := NewFont(14, 2, 17, 14)
	set.Add(a)
	set.Add(b)
	set.Add(c)
	if set.Closest(14, 1) != b {
		t.Fatal("exact match expected at scale 1")
	}
	if set.Closest(14, 2) != c {
		t.Fatal("hi-dpi atlas expected at scale 2")
	}
	if set.Closest(12.5, 1) != a {
		t.Fatal("nearest size expected")
	}
}
