package sprite

import (
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/material"
	"aego/render/shader"
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
	Material *material.Material
	Lanes    *[16]float32
}

// The sprite blend modes are the shader ABI's, so a material's blend picks the same pipeline state.
const (
	_ = uint(BlendOpaque) - uint(shader.BlendOpaque)
	_ = uint(shader.BlendOpaque) - uint(BlendOpaque)
	_ = uint(BlendAdditive) - uint(shader.BlendAdditive)
	_ = uint(shader.BlendAdditive) - uint(BlendAdditive)
)

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
	return d.Tint
}
