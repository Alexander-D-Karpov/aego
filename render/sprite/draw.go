package sprite

import (
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/texture"
)

type BlendMode uint8

const (
	BlendPremultiplied BlendMode = iota
	BlendOpaque
	BlendAdditive
	blendCount
)

type Draw struct {
	Region   texture.Region
	Position mathx.Vec2
	Size     mathx.Vec2
	Scale    mathx.Vec2
	Rotation float32
	Origin   mathx.Vec2
	Z        float32
	Tint     mathx.Color
	Layer    int16
	Order    int16
	Blend    BlendMode
	Sampler  gpu.SamplerHandle
	FlipX    bool
	FlipY    bool
}

func (d *Draw) scale() mathx.Vec2 {
	if d.Scale.X == 0 && d.Scale.Y == 0 {
		return mathx.Vec2{X: 1, Y: 1}
	}
	return d.Scale
}

func (d *Draw) WorldSize(pixelsPerUnit float32) mathx.Vec2 {
	base := d.Size
	if base.X == 0 && base.Y == 0 {
		if pixelsPerUnit <= 0 {
			pixelsPerUnit = 1
		}
		base = d.Region.Size().Scale(1 / pixelsPerUnit)
	}
	return base.Mul(d.scale())
}

func (d *Draw) tint() mathx.Color {
	c := d.Tint
	if c == (mathx.Color{}) {
		return mathx.Color{R: 1, G: 1, B: 1, A: 1}
	}
	return c
}
