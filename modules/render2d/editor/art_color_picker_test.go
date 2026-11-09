package editor

import (
	"testing"

	"aego/art"
	"aego/core/mathx"
)

func TestArtColorPickerHSVAndAlphaRoundTrip(t *testing.T) {
	for _, color := range []art.Color{
		{R: 255, G: 0, B: 0, A: 0}, {R: 11, G: 83, B: 201, A: 89},
		{R: 255, G: 255, B: 255, A: 255}, {R: 48, G: 48, B: 48, A: 16},
		{R: 0, G: 0, B: 0, A: 99},
	} {
		h, s, v := artColorHSV(color, .73, .66)
		got := artHSVColor(h, s, v, color.A)
		if got != color {
			t.Fatalf("HSV round trip changed %+v into %+v", color, got)
		}
	}
	h, s, v := artColorHSV(art.Color{A: 255}, .42, .67)
	if h != .42 || s != .67 || v != 0 {
		t.Fatalf("black lost its chosen hue and saturation: %g %g %g", h, s, v)
	}
}

func TestArtResponsiveGridsRemainInsideDock(t *testing.T) {
	for _, width := range []float32{1, 24, 72, 136, 192, 400, 900} {
		for _, count := range []int{1, 14, 16, 256} {
			columns, size := artGridSize(width, count, 32, 44)
			if columns < 1 || columns > count || size <= 0 || size > 44 || float32(columns)*size+float32(columns-1)*4 > width+.01 {
				t.Fatalf("width=%g count=%d: columns=%d size=%g", width, count, columns, size)
			}
		}
	}
	if columns, _ := artGridSize(0, 14, 32, 44); columns != 0 {
		t.Fatal("empty dock should not lay out controls")
	}
}

func TestColorPickerDoesNotAlterPaletteOrDirtyDrawing(t *testing.T) {
	p := artActionFixture(t)
	before := p.model.Session.Document.Info()
	history := p.model.History.Cursor()
	dirty := p.doc.Dirty()
	p.setForegroundColor(mathx.ColorRGBA8(41, 102, 183, 73), true)
	after := p.model.Session.Document.Info()
	if after.Revision != before.Revision || p.model.History.Cursor() != history || len(after.Palette) != len(before.Palette) || p.doc.Dirty() != dirty {
		t.Fatal("selecting a foreground color changed the drawing or its palette")
	}
	if got := artColorBytes(p.view.color); got != (art.Color{R: 41, G: 102, B: 183, A: 73}) {
		t.Fatalf("foreground lost RGBA bytes: %+v", got)
	}
}
