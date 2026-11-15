package text

import (
	"testing"

	"aego/core/mathx"
)

func TestGlyphVerticalBoundsRemainConservative(t *testing.T) {
	f := NewFont(14, 2, 18, 14)
	f.SetGlyph('A', FontGlyph{Offset: mathx.Vec2{Y: -42}, Size: mathx.Vec2{Y: 60}})
	f.SetGlyph('?', FontGlyph{Offset: mathx.Vec2{Y: 5}, Size: mathx.Vec2{Y: 50}})
	f.SetFallback('?')
	f.SetGlyph('A', FontGlyph{Offset: mathx.Vec2{Y: -2}, Size: mathx.Vec2{Y: 8}})
	lo, hi := f.GlyphVerticalBounds()
	if lo > -42 || hi < 55 {
		t.Fatalf("lost conservative bounds after replacing a glyph: %v..%v", lo, hi)
	}
	f.SetGlyph('界', FontGlyph{Offset: mathx.Vec2{Y: -70}, Size: mathx.Vec2{Y: 160}})
	lo, hi = f.GlyphVerticalBounds()
	if lo > -70 || hi < 90 {
		t.Fatalf("new non-ASCII glyph is outside bounds: %v..%v", lo, hi)
	}
}
