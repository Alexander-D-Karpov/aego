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
