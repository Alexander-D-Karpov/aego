package culling

import (
	"aego/core/mathx"
	"aego/render/sprite"
)

func Bounds(d *sprite.Draw, pixelsPerUnit float32) mathx.AABB {
	size := d.WorldSize(pixelsPerUnit)
	origin := d.Origin
	minX := -origin.X * size.X
	minY := -origin.Y * size.Y
	maxX := minX + size.X
	maxY := minY + size.Y
	if d.Rotation == 0 {
		return mathx.AABB{
			Min: mathx.Vec2{X: d.Position.X + minX, Y: d.Position.Y + minY},
			Max: mathx.Vec2{X: d.Position.X + maxX, Y: d.Position.Y + maxY},
		}
	}
	s, c := mathx.Sin(d.Rotation), mathx.Cos(d.Rotation)
	corners := [4]mathx.Vec2{
		{X: minX, Y: minY},
		{X: maxX, Y: minY},
		{X: maxX, Y: maxY},
		{X: minX, Y: maxY},
	}
	lo := mathx.Vec2{X: mathx.Pi * 1e9, Y: mathx.Pi * 1e9}
	hi := lo.Neg()
	for _, p := range corners {
		r := mathx.Vec2{X: p.X*c - p.Y*s + d.Position.X, Y: p.X*s + p.Y*c + d.Position.Y}
		lo.X, lo.Y = min(lo.X, r.X), min(lo.Y, r.Y)
		hi.X, hi.Y = max(hi.X, r.X), max(hi.Y, r.Y)
	}
	return mathx.AABB{Min: lo, Max: hi}
}

func Visible(bounds mathx.AABB, d *sprite.Draw, pixelsPerUnit float32) bool {
	return bounds.Intersects(Bounds(d, pixelsPerUnit))
}

func Cull(visible mathx.AABB, draws []sprite.Draw, pixelsPerUnit float32, out []uint32) []uint32 {
	out = out[:0]
	for i := range draws {
		if Visible(visible, &draws[i], pixelsPerUnit) {
			out = append(out, uint32(i))
		}
	}
	return out
}
