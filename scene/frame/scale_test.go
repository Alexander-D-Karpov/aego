package frame

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/gpu/null"
	"aego/render/rt"
	"aego/render/view"
)

func TestRenderScaleTarget(t *testing.T) {
	dev := null.New()
	r := New()
	r.SetRenderScale(0.5)
	r.scaleTarget(dev, 1280, 720, 1)
	if r.RenderScale() != 0.5 {
		t.Fatalf("render scale %v", r.RenderScale())
	}
	r.Prepare(1280, 720, 1)
	if r.PixelScale() != 0.5 || r.pixelW != 640 || r.pixelH != 360 {
		t.Fatalf("scaled frame %dx%d at %v", r.pixelW, r.pixelH, r.PixelScale())
	}
	out := r.Output()
	if !out.Target.IsZero() || out.W != 1280 || out.H != 720 || out.Scale != 1 {
		t.Fatalf("output %+v, want the full backbuffer", out)
	}
	ext := gpu.TextureHandle{}
	r.SetTarget(ext, 800, 600)
	r.scaleTarget(dev, 1280, 720, 1)
	if r.RenderScale() != 1 {
		t.Fatal("a host target was scaled")
	}
	r.Close()
}

func TestOnlyFullOpaqueCameraClearsOwnedPoolPadding(t *testing.T) {
	pool := rt.New(null.New(), gpu.TexSRGBA8, "padded camera")
	defer pool.Close()
	target, err := pool.Resize(nil, 91, 73, false)
	if err != nil {
		t.Fatal(err)
	}
	r := New()
	r.SetPooledTarget(target)
	r.Prepare(1, 1, 1)
	v := view.View{Target: target.Texture, TargetW: target.BucketW, TargetH: target.BucketH, Viewport: r.Viewport(mathx.Rect{}), DoClear: true, Clear: mathx.Color{A: 1}}
	test := func(v view.View, want bool) {
		t.Helper()
		r.AddView(v, 0)
		got := r.Views()[len(r.Views())-1]
		if got.ClearWholeTarget != want || got.Viewport != v.Viewport || (want && got.ClearRegion() != (mathx.Rect{W: 128, H: 128})) {
			t.Fatalf("camera %v / opacity %v: clear %v, whole=%v; want whole=%v", v.Viewport, v.Clear.A, got.ClearRegion(), got.ClearWholeTarget, want)
		}
	}
	test(v, true)
	split := v
	split.Viewport.X, split.Viewport.W = 45, 46
	test(split, false)
	transparent := v
	transparent.Clear.A = 0
	test(transparent, false)
	keep := v
	keep.DoClear = false
	test(keep, false)
	r.SetTarget(target.Texture, target.BucketW, target.BucketH)
	r.SetViewportSize(target.W, target.H)
	test(v, false) // An arbitrary host's sub-rectangle doesn't own the padding.
	pool.Release(target)
}
