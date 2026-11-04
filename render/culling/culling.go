package culling

import (
	"aego/core/mathx"
	"aego/render/sprite"
)

func Bounds(d *sprite.Draw, pixelsPerUnit float32) mathx.AABB2 {
	size, origin := d.WorldSize(pixelsPerUnit), d.Origin
	if d.Region.Trimmed() {
		_, size, origin = d.Geometry(pixelsPerUnit)
	}
	if !d.HasLinear && d.Rotation == 0 {
		// Preserve the common axis-aligned Unlit path. Explicit signed sizes
		// still need ordered endpoints, unlike the former positive-only test.
		x0, y0 := d.Position.X-origin.X*size.X, d.Position.Y-origin.Y*size.Y
		x1, y1 := x0+size.X, y0+size.Y
		if size.X < 0 {
			x0, x1 = x1, x0
		}
		if size.Y < 0 {
			y0, y1 = y1, y0
		}
		return mathx.AABB2{Min: mathx.Vec2{X: x0, Y: y0}, Max: mathx.Vec2{X: x1, Y: y1}}
	}
	basis := d.Linear
	if !d.HasLinear {
		s, c := float32(0), float32(1)
		if d.Rotation != 0 {
			s, c = mathx.Sin(d.Rotation), mathx.Cos(d.Rotation)
		}
		basis = [4]float32{c, s, -s, c}
	}
	// Transform center/extents rather than assuming positive scales. This is
	// exact for affine rectangles, including shear, reflection and zero axes.
	cx, cy := (0.5-origin.X)*size.X, (0.5-origin.Y)*size.Y
	ex, ey := mathx.Abs(size.X)*0.5, mathx.Abs(size.Y)*0.5
	center := mathx.Vec2{X: d.Position.X + basis[0]*cx + basis[2]*cy, Y: d.Position.Y + basis[1]*cx + basis[3]*cy}
	extent := mathx.Vec2{X: mathx.Abs(basis[0])*ex + mathx.Abs(basis[2])*ey, Y: mathx.Abs(basis[1])*ex + mathx.Abs(basis[3])*ey}
	return mathx.AABB2{Min: center.Sub(extent), Max: center.Add(extent)}
}

func Visible(bounds mathx.AABB2, d *sprite.Draw, pixelsPerUnit float32) bool {
	return bounds.Intersects(Bounds(d, pixelsPerUnit))
}

func Cull(visible mathx.AABB2, draws []sprite.Draw, pixelsPerUnit float32, out []uint32) []uint32 {
	out = out[:0]
	for i := range draws {
		if Visible(visible, &draws[i], pixelsPerUnit) {
			out = append(out, uint32(i))
		}
	}
	return out
}
