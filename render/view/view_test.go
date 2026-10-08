package view

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
)

const eps = 1e-3

func testView() View {
	v := View{
		Viewport: mathx.Rect{W: 800, H: 600},
		TargetW:  800,
		TargetH:  600,
		Camera:   Camera2D{Position: mathx.Vec2{X: 12, Y: -7}, Rotation: 0.6, Zoom: 32},
	}
	v.Normalize(800, 600, 1)
	return v
}

func TestScreenWorldRoundTrip(t *testing.T) {
	v := testView()
	for _, p := range []mathx.Vec2{{}, {X: 3, Y: 4}, {X: -18, Y: 22}} {
		back := v.ScreenToWorld(v.WorldToScreen(p))
		if !back.NearlyEqual(p, eps) {
			t.Fatalf("%v -> %v", p, back)
		}
	}
}

func TestVisibleBoundsContainsCorners(t *testing.T) {
	v := testView()
	b := v.VisibleBounds()
	vp := v.LetterboxViewport()
	for _, s := range []mathx.Vec2{
		{X: vp.X, Y: vp.Y},
		{X: vp.X + vp.W, Y: vp.Y},
		{X: vp.X, Y: vp.Y + vp.H},
		{X: vp.X + vp.W, Y: vp.Y + vp.H},
	} {
		w := v.ScreenToWorld(s)
		if !b.Contains(w) {
			t.Fatalf("corner %v not in bounds %v", w, b)
		}
	}
}

func TestPixelPerfectIntegerZoomAndLetterbox(t *testing.T) {
	v := View{
		Viewport: mathx.Rect{W: 800, H: 600},
		Camera:   Camera2D{Zoom: 1, Position: mathx.Vec2{X: 10.37, Y: -3.21}},
		Scale:    ScalePixelPerfect,
		Virtual:  mathx.Vec2{X: 320, Y: 180},
	}
	v.Normalize(800, 600, 1)
	z := v.EffectiveZoom()
	if z != float32(int(z)) || z < 1 {
		t.Fatalf("zoom not integer: %f", z)
	}
	lb := v.LetterboxViewport()
	if lb.W > 800.001 || lb.H > 600.001 {
		t.Fatalf("letterbox exceeds viewport: %+v", lb)
	}
	if mathx.Abs(lb.W/lb.H-320.0/180.0) > 1e-3 {
		t.Fatalf("letterbox aspect wrong: %+v", lb)
	}
	p := v.cameraPosition()
	if mathx.Abs(p.X*z-float32(int(p.X*z))) > 1e-3 {
		t.Fatalf("camera not snapped to pixel grid: %v", p)
	}
}

func TestProjectionFlipsOnlyForTargets(t *testing.T) {
	caps := gpu.Capabilities{OriginBottomLeft: true}
	v := testView()
	window := v.ProjectionMatrix(caps)
	v.Target = gpu.TextureHandle{Index: 1, Gen: 1}
	target := v.ProjectionMatrix(caps)
	for i := range window {
		want := window[i]
		if i == 5 || i == 13 {
			want = -want
		}
		if mathx.Abs(target[i]-want) > 1e-5 {
			t.Fatalf("projection differs beyond a Y flip at %d: %f vs %f", i, target[i], window[i])
		}
	}
	flat := gpu.Capabilities{}
	if v.ProjectionMatrix(flat) != window {
		t.Fatal("backends without OriginBottomLeft must not flip")
	}
}

func TestScreenViewMapsPixels(t *testing.T) {
	v := ScreenView(800, 600, 2)
	v.Normalize(800, 600, 2)
	if got := v.WorldToScreen(mathx.Vec2{}); !got.NearlyEqual(mathx.Vec2{}, eps) {
		t.Fatalf("origin -> %v", got)
	}
	if got := v.WorldToScreen(mathx.Vec2{X: 400, Y: 300}); !got.NearlyEqual(mathx.Vec2{X: 800, Y: 600}, eps) {
		t.Fatalf("bottom-right -> %v", got)
	}
}
