package frame

import (
	"testing"

	"aego/gpu"
	"aego/gpu/null"
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
