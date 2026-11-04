package culling

import (
	"testing"

	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/texture"
)

func draw(x, y, w, h, rot float32) sprite.Draw {
	return sprite.Draw{
		Region:   texture.Whole(gpu.TextureHandle{Index: 1, Gen: 1}, 16, 16),
		Position: mathx.Vec2{X: x, Y: y},
		Size:     mathx.Vec2{X: w, Y: h},
		Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
		Rotation: rot,
	}
}

func TestBoundsAxisAligned(t *testing.T) {
	d := draw(10, 20, 4, 2, 0)
	b := Bounds(&d, 1)
	if b.Min.X != 8 || b.Min.Y != 19 || b.Max.X != 12 || b.Max.Y != 21 {
		t.Fatalf("bounds = %+v", b)
	}
}

func TestBoundsRotated(t *testing.T) {
	d := draw(0, 0, 2, 2, mathx.Pi/4)
	b := Bounds(&d, 1)
	want := mathx.Sqrt(2)
	if mathx.Abs(b.Max.X-want) > 1e-3 || mathx.Abs(b.Min.X+want) > 1e-3 {
		t.Fatalf("rotated bounds = %+v, want ±%f", b, want)
	}
	d.Rotation = mathx.Pi / 2
	b = Bounds(&d, 1)
	if mathx.Abs(b.Max.X-1) > 1e-3 {
		t.Fatalf("quarter turn of a square must keep extents: %+v", b)
	}
}

func TestBoundsUsesPixelsPerUnit(t *testing.T) {
	d := draw(0, 0, 0, 0, 0)
	d.Size = mathx.Vec2{}
	b := Bounds(&d, 16)
	if mathx.Abs(b.Size().X-1) > 1e-3 {
		t.Fatalf("16px region at ppu=16 must be 1 unit: %+v", b)
	}
	b = Bounds(&d, 8)
	if mathx.Abs(b.Size().X-2) > 1e-3 {
		t.Fatalf("ppu=8 must double the size: %+v", b)
	}
}

func TestCull(t *testing.T) {
	visible := mathx.Rect{X: -5, Y: -5, W: 10, H: 10}.ToAABB()
	draws := []sprite.Draw{
		draw(0, 0, 1, 1, 0),
		draw(100, 0, 1, 1, 0),
		draw(-4.9, 0, 1, 1, 0),
		draw(-100, -100, 1, 1, 0),
	}
	out := Cull(visible, draws, 1, make([]uint32, 0, 4))
	if len(out) != 2 || out[0] != 0 || out[1] != 2 {
		t.Fatalf("cull = %v", out)
	}
}

func TestAffineBoundsMatchAllPresentationCornersWithTrimAndFlips(t *testing.T) {
	m := mathx.Mat3TRS(mathx.Vec2{X: 19, Y: -7}, .4, mathx.Vec2{X: -3, Y: 2}).Mul(mathx.Mat3TRS(mathx.Vec2{}, .7, mathx.Vec2{X: 1, Y: 1}))
	d := draw(0, 0, 8, 6, 0)
	d.Region.SourceW, d.Region.SourceH, d.Region.W, d.Region.H, d.Region.TrimX, d.Region.TrimY = 8, 6, 3, 2, 1, 1
	d.SetTransform(m)
	for _, flip := range []bool{false, true} {
		d.FlipX, d.FlipY = flip, flip
		got := Bounds(&d, 1)
		x0, y0 := float32(-3), float32(-2)
		if flip {
			x0, y0 = 0, 0
		}
		lo, hi := m.Apply(mathx.Vec2{X: x0, Y: y0}), m.Apply(mathx.Vec2{X: x0, Y: y0})
		for _, p := range []mathx.Vec2{{X: x0 + 3, Y: y0}, {X: x0 + 3, Y: y0 + 2}, {X: x0, Y: y0 + 2}} {
			v := m.Apply(p)
			lo.X, lo.Y, hi.X, hi.Y = min(lo.X, v.X), min(lo.Y, v.Y), max(hi.X, v.X), max(hi.Y, v.Y)
		}
		if mathx.Abs(got.Min.X-lo.X) > 1e-5 || mathx.Abs(got.Min.Y-lo.Y) > 1e-5 || mathx.Abs(got.Max.X-hi.X) > 1e-5 || mathx.Abs(got.Max.Y-hi.Y) > 1e-5 {
			t.Fatalf("flip=%v affine bounds %v, want %v..%v", flip, got, lo, hi)
		}
	}
}

func TestBoundsNegativeLegacyScaleAndZeroAffineRemainOrdered(t *testing.T) {
	d := draw(10, 20, 4, 2, 0)
	d.Scale = mathx.Vec2{X: -2, Y: -3}
	b := Bounds(&d, 1)
	if b.Min != (mathx.Vec2{X: 6, Y: 17}) || b.Max != (mathx.Vec2{X: 14, Y: 23}) {
		t.Fatalf("negative scale reversed AABB endpoints: %v", b)
	}
	d.HasLinear, d.Linear = true, [4]float32{}
	b = Bounds(&d, 1)
	if b.Min != d.Position || b.Max != d.Position {
		t.Fatal("degenerate full transform must produce a point, not identity")
	}
}
