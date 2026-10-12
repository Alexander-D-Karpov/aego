package render3d

import (
	"testing"

	"aego/core/mathx"
	"aego/render/view"
)

func TestCameraViewProjection(t *testing.T) {
	vp := mathx.Rect{W: 200, H: 100}
	cam := DefaultCamera3D()
	if v := cameraView(&cam, mathx.Mat4Identity(), vp); v.Projection != view.Perspective3D {
		t.Fatalf("projection = %v", v.Projection)
	}
	cam.Orthographic, cam.OrthoSize = true, 4
	v := cameraView(&cam, mathx.Mat4Translate(mathx.Vec3{Z: 10}), vp)
	if v.Projection != view.Ortho3D || v.OrthoSize != 4 {
		t.Fatalf("view = %v size %v", v.Projection, v.OrthoSize)
	}
	v.Normalize(200, 100, 1)
	near, _ := v.WorldToScreen3(mathx.Vec3{X: 1})
	far, _ := v.WorldToScreen3(mathx.Vec3{X: 1, Z: -100})
	if near.Sub(far).Len() > 1e-3 {
		t.Fatalf("ortho camera has perspective: %v vs %v", near, far)
	}
}
