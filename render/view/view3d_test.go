package view

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
)

func perspectiveTestView() (View, mathx.Mat4) {
	pos := mathx.Vec3{X: 1, Y: 2, Z: 5}
	eye := mathx.Mat4TRS(pos, mathx.QuatLookAt(pos.Neg(), mathx.Vec3{Y: 1}), mathx.Vec3{X: 1, Y: 1, Z: 1})
	v := PerspectiveView(eye, mathx.Deg2Rad(60), 0.1, 100, mathx.Rect{W: 800, H: 600})
	v.Normalize(800, 600, 1)
	return v, eye
}

func TestScreenToRayCenterIsForward(t *testing.T) {
	v, eye := perspectiveTestView()
	ray := v.ScreenToRay(mathx.Vec2{X: 400, Y: 300})
	want := eye.TransformDir(mathx.Vec3{Z: -1}).Normalized()
	if !ray.Dir.NearlyEqual(want, 1e-3) {
		t.Fatalf("dir = %v, want %v", ray.Dir, want)
	}
	if ray.Origin.Sub(eye.Translation()).Len() > 0.2 {
		t.Fatalf("ray must start at the near plane: %v", ray.Origin)
	}
}

func TestWorldToScreen3RoundTrip(t *testing.T) {
	v, _ := perspectiveTestView()
	for _, p := range []mathx.Vec2{{X: 400, Y: 300}, {X: 10, Y: 20}, {X: 790, Y: 580}, {X: 120, Y: 450}} {
		w := v.ScreenToRay(p).At(7)
		back, ok := v.WorldToScreen3(w)
		if !ok || !back.NearlyEqual(p, 0.05) {
			t.Fatalf("%v -> %v (%v)", p, back, ok)
		}
	}
	if _, ok := v.WorldToScreen3(mathx.Vec3{X: 2, Y: 4, Z: 10}); ok {
		t.Fatal("a point behind the camera must not project")
	}
}

func TestDepthAndWinding(t *testing.T) {
	v, _ := perspectiveTestView()
	caps := gpu.Capabilities{OriginBottomLeft: true}
	if !v.UsesDepth() || v.Flipped(caps) {
		t.Fatal("window perspective view must use the window depth and stay unflipped")
	}
	v.Target = gpu.TextureHandle{Index: 1, Gen: 1}
	if v.UsesDepth() || !v.Flipped(caps) {
		t.Fatal("offscreen view without a depth target must not use depth and must flip")
	}
	v.DepthTarget = gpu.TextureHandle{Index: 2, Gen: 1}
	if !v.UsesDepth() {
		t.Fatal("offscreen view with a depth target must use depth")
	}
	if ScreenView(64, 64, 1).UsesDepth() {
		t.Fatal("2D views never use depth")
	}
}

func TestFrustumFollowsEye(t *testing.T) {
	v, _ := perspectiveTestView()
	f := v.Frustum()
	if !f.ContainsPoint(mathx.Vec3{}) {
		t.Fatal("origin is in front of the camera")
	}
	if f.ContainsPoint(mathx.Vec3{X: 2, Y: 4, Z: 10}) {
		t.Fatal("point behind the camera inside the frustum")
	}
}

func TestOrthoViewMatrix4MatchesMat3(t *testing.T) {
	v := testView()
	p := mathx.Vec2{X: 3, Y: -4}
	a := v.ViewMatrix().Apply(p)
	b := v.ViewMatrix4().TransformPoint(mathx.Vec3{X: p.X, Y: p.Y})
	if !b.NearlyEqual(mathx.Vec3{X: a.X, Y: a.Y}, 1e-4) {
		t.Fatalf("ortho ViewMatrix4 diverges from ViewMatrix: %v vs %v", b, a)
	}
}

func TestOrthoViewIgnoresDepth(t *testing.T) {
	eye := mathx.Mat4Translate(mathx.Vec3{Z: 10})
	v := OrthoView(eye, 5, 0.1, 100, mathx.Rect{W: 200, H: 100})
	v.Normalize(200, 100, 1)
	if !v.Is3D() || !v.UsesDepth() {
		t.Fatal("ortho view must be a 3D view with depth")
	}
	a, okA := v.WorldToScreen3(mathx.Vec3{X: 2, Y: 1})
	b, okB := v.WorldToScreen3(mathx.Vec3{X: 2, Y: 1, Z: -50})
	if !okA || !okB {
		t.Fatal("points in front of the camera must project")
	}
	if a.Sub(b).Len() > 1e-3 {
		t.Fatalf("depth changed the pixel: %v vs %v", a, b)
	}
	want := mathx.Vec2{X: 120, Y: 40}
	if a.Sub(want).Len() > 1e-3 {
		t.Fatalf("pixel = %v, want %v", a, want)
	}
	ray := v.ScreenToRay(a)
	if ray.Dir.Sub(mathx.Vec3{Z: -1}).Len() > 1e-4 {
		t.Fatalf("ray dir = %v", ray.Dir)
	}
}
