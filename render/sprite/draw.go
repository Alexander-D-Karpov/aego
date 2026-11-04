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
	Profile          *SubmissionStats
	VisibilityLayers uint64
	PixelCoverage    bool
	LightSet         uint32
	Region           texture.Region
	Position         mathx.Vec2
	Size             mathx.Vec2
	Scale            mathx.Vec2
	Rotation         float32
	Origin           mathx.Vec2
	Z                float32
	Tint             mathx.Color
	Layer            int16
	Order            int16
	Blend            BlendMode
	Sampler          gpu.SamplerHandle
	FlipX            bool
	FlipY            bool
	Material         *material.Material
	Lanes            *[16]float32
	// Linear replaces Scale and Rotation when HasLinear is true. It is the
	// complete local-to-world 2x2 basis in column-major order, including shear
	// and reflection. Position is still the translation.
	Linear    [4]float32
	HasLinear bool
	// Normal uses its own atlas region in source-canvas coordinates. Its UVs
	// are not flipped again: the albedo's local coordinates carry FlipX/FlipY.
	Normal   texture.Region
	Emission texture.Emission
	Lighting bool
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
	if d.HasLinear {
		return base
	}
	return base.Mul(d.scale())
}

// SetTransform preserves the exact presentation basis when it cannot be
// represented by the legacy positive-scale rotation path. Ordinary sprites
// keep the original 52-byte stream without an auxiliary upload.
func (d *Draw) SetTransform(m mathx.Mat3) {
	d.Position = mathx.Vec2{X: m[6], Y: m[7]}
	d.Linear = [4]float32{m[0], m[1], m[3], m[4]}
	sx := mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
	sy := mathx.Sqrt(m[3]*m[3] + m[4]*m[4])
	det := m[0]*m[4] - m[1]*m[3]
	dot := m[0]*m[3] + m[1]*m[4]
	d.HasLinear = d.Lighting || !(det > 0 && sx > 0 && sy > 0 && mathx.Abs(dot) <= 1e-6*sx*sy)
	if d.HasLinear {
		d.Scale = mathx.Vec2{X: 1, Y: 1}
		d.Rotation = 0
		return
	}
	d.Scale = mathx.Vec2{X: sx, Y: sy}
	d.Rotation = mathx.Atan2(m[1], m[0])
}

// Basis is the complete local-to-world linear transform. Lighting promotes
// legacy draws to this basis so nonuniform scale also reaches the normal ABI.
func (d *Draw) Basis() [4]float32 {
	if d.HasLinear {
		return d.Linear
	}
	scale := d.scale()
	s, c := float32(0), float32(1)
	if d.Rotation != 0 {
		s, c = mathx.Sin(d.Rotation), mathx.Cos(d.Rotation)
	}
	return [4]float32{c * scale.X, s * scale.X, -s * scale.Y, c * scale.Y}
}

func (d *Draw) tint() mathx.Color {
	return d.Tint
}

// Geometry keeps the simulation transform and original canvas size unchanged,
// while the submitted quad covers only the nontransparent imported rectangle.
func (d *Draw) Geometry(pixelsPerUnit float32) (texture.Region, mathx.Vec2, mathx.Vec2) {
	reg := d.Region
	if d.FlipX {
		reg = reg.FlippedX()
	}
	if d.FlipY {
		reg = reg.FlippedY()
	}
	size, origin := d.WorldSize(pixelsPerUnit), d.Origin
	if reg.Trimmed() {
		sx, sy := float32(reg.W)/float32(reg.SourceW), float32(reg.H)/float32(reg.SourceH)
		origin.X = (origin.X*float32(reg.SourceW) - float32(reg.TrimX)) / float32(reg.W)
		origin.Y = (origin.Y*float32(reg.SourceH) - float32(reg.TrimY)) / float32(reg.H)
		size.X *= sx
		size.Y *= sy
	}
	return reg, size, origin
}
