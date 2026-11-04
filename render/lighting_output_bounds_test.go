package render

import (
	"testing"

	"aego/core/mathx"
	"aego/render/sprite"
	"aego/render/view"
)

func TestResolveRectContainsReceiverAtCameraRotations(t *testing.T) {
	target := mathx.Rect{W: 1280, H: 720}
	f := ViewFrame{view: view.ScreenView(1280, 720, 1), resolveBounds: mathx.AABB2{Min: mathx.Vec2{X: 200, Y: 105}, Max: mathx.Vec2{X: 246, Y: 172}}}
	for _, r := range []float32{0, .19, -.27} {
		f.view.Camera.Rotation = r
		cut, ok := f.resolveRect(target)
		if !ok {
			t.Fatal("small bounded receiver fell back")
		}
		for _, x := range []float32{200, 246} {
			for _, y := range []float32{105, 172} {
				p := f.view.WorldToScreen(mathx.Vec2{X: x, Y: y})
				if target.Contains(p) && !cut.Contains(p) {
					t.Fatalf("%v outside %v", p, cut)
				}
			}
		}
	}
	f.resolveBounds = mathx.AABB2{Min: mathx.Vec2{X: -1e20, Y: -3}, Max: mathx.Vec2{X: 1e20, Y: 3}}
	if _, ok := f.resolveRect(target); ok {
		t.Fatal("imprecise world extent used as a scissor")
	}
}

func TestResolveBackgroundAmbiguousByteFallsBack(t *testing.T) {
	if _, ok := resolveBackground(mathx.Color{A: .5}, 1); ok {
		t.Fatal("transparent clear qualified")
	}
	// Inverse Reinhard at the sRGB 100/101 boundary, so half-float storage may choose either code.
	linear := mathx.SRGBToLinear(100.5 / 255)
	v := linear / (1 - linear)
	if _, ok := resolveBackground(mathx.Color{R: v, A: 1}, 1); ok {
		t.Fatal("ambiguous clear qualified")
	}
}

func TestSparseResolveRejectsCancellationBeforeBounds(t *testing.T) {
	f := ViewFrame{view: view.ScreenView(1280, 720, 1)}
	d := sprite.Draw{Position: mathx.Vec2{X: 105000000}, Origin: mathx.Vec2{X: 5000000}, Size: mathx.Vec2{X: 21, Y: 1}}
	if f.stableSpriteBounds(&d) {
		t.Fatal("canceling vertex operands qualified for sparse resolve")
	}
}
