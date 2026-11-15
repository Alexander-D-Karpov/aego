package render2d

import (
	"slices"

	"aego/asset"
	"aego/core/mathx"
	"aego/render/lighting"
	"aego/scene"
)

type Light2D struct {
	Type             lighting.Type            `aego:"type"`
	Color            mathx.Color              `aego:"color"`
	Intensity        float32                  `aego:"intensity,min=0"`
	Range            float32                  `aego:"range,min=0.001"`
	Height           float32                  `aego:"height,min=0"`
	InnerAngle       float32                  `aego:"innerAngle,min=0,max=3.1415927"`
	OuterAngle       float32                  `aego:"outerAngle,min=0,max=3.1415927"`
	Elevation        float32                  `aego:"elevation,min=0.001,max=1.5707964"`
	Falloff          float32                  `aego:"falloff,min=0"`
	Cookie           asset.Ref[asset.Texture] `aego:"cookie,asset=texture,omitempty"`
	Layers           int64                    `aego:"layers"`
	ShadowCasterMask int64                    `aego:"shadowCasterMask"`
	Shadows          bool                     `aego:"shadows"`
	ShadowSoftness   float32                  `aego:"shadowSoftness,min=0"`
	Priority         int32                    `aego:"priority"`
}

type OccluderSource uint8

const (
	OccluderPolygon OccluderSource = iota
	OccluderShapeProvider
	OccluderSpriteAlpha
)

type LightOccluder2D struct {
	Source    OccluderSource    `aego:"source"`
	Polygon   []mathx.Vec2      `aego:"polygon"`
	Closed    bool              `aego:"closed"`
	CullMode  lighting.CullMode `aego:"cullMode"`
	Layers    int64             `aego:"layers"`
	ShapeNode scene.NodeRef     `aego:"shapeNode,omitempty"`
	Shape     string            `aego:"shape,omitempty"`
}

func cloneLightOccluder(dst, src *LightOccluder2D) {
	*dst = *src
	dst.Polygon = slices.Clone(src.Polygon)
}

type LightingEnvironment2D struct {
	Camera            scene.NodeRef      `aego:"camera,omitempty"`
	AmbientColor      mathx.Color        `aego:"ambientColor"`
	AmbientIntensity  float32            `aego:"ambientIntensity,min=0"`
	ColorMode         lighting.ColorMode `aego:"colorMode"`
	Exposure          float32            `aego:"exposure,min=0.001"`
	ShadowResolution  int32              `aego:"shadowResolution,min=64,max=2048"`
	MaxShadowedLights int32              `aego:"maxShadowedLights,min=0,max=16"`
}

func DefaultLight2D() Light2D {
	return Light2D{Type: lighting.Point, Color: mathx.Color{R: 1, G: 1, B: 1, A: 1}, Intensity: 1, Range: 5, Height: 1,
		InnerAngle: mathx.Pi / 8, OuterAngle: mathx.Pi / 4, Elevation: mathx.Pi / 4, Falloff: 2, Layers: -1, ShadowCasterMask: -1}
}

func DefaultLightOccluder2D() LightOccluder2D {
	return LightOccluder2D{Closed: true, Layers: -1, Polygon: []mathx.Vec2{{X: -.5, Y: -.5}, {X: .5, Y: -.5}, {X: .5, Y: .5}, {X: -.5, Y: .5}}}
}

func DefaultLightingEnvironment2D() LightingEnvironment2D {
	e := lighting.DefaultEnvironment()
	return LightingEnvironment2D{AmbientColor: e.Ambient, AmbientIntensity: e.AmbientIntensity, ColorMode: e.ColorMode, Exposure: e.Exposure,
		ShadowResolution: int32(e.ShadowResolution), MaxShadowedLights: int32(e.MaxShadowedLights)}
}

func (l *Light2D) descriptor() lighting.Light {
	return lighting.Light{Type: l.Type, Direction: mathx.Vec2{X: 1}, Color: l.Color, Intensity: l.Intensity, Range: l.Range, Height: l.Height,
		InnerAngle: l.InnerAngle, OuterAngle: l.OuterAngle, Elevation: l.Elevation, Falloff: l.Falloff, Layers: uint64(l.Layers),
		ShadowCasterMask: uint64(l.ShadowCasterMask), Shadows: l.Shadows, ShadowSoftness: l.ShadowSoftness, Priority: l.Priority}
}

func (e *LightingEnvironment2D) descriptor() lighting.Environment {
	return lighting.Environment{Ambient: e.AmbientColor, AmbientIntensity: e.AmbientIntensity, ColorMode: e.ColorMode,
		Exposure: e.Exposure, ShadowResolution: int(e.ShadowResolution), MaxShadowedLights: int(e.MaxShadowedLights)}
}
