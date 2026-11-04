package render2d

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/view"
)

type SpriteRenderer struct {
	Sprite   asset.Ref[asset.Texture]  `aego:"sprite,asset=texture"`
	Material asset.Ref[asset.Material] `aego:"material,asset=material,omitempty"`
	Layer    int32                     `aego:"layer"`
	Order    int32                     `aego:"order"`
	Tint     mathx.Color               `aego:"tint"`
	Origin   mathx.Vec2                `aego:"origin"`
	Size     mathx.Vec2                `aego:"size"`
	FlipX    bool                      `aego:"flipX"`
	FlipY    bool                      `aego:"flipY"`
	Blend    sprite.BlendMode          `aego:"blend"`
	Filter   gpu.FilterMode            `aego:"filter"`
}

type Camera2D struct {
	Zoom       float32        `aego:"zoom,min=0.01"`
	Viewport   mathx.Rect     `aego:"viewport"`
	Mask       int64          `aego:"mask"`
	Order      int32          `aego:"order"`
	Clear      bool           `aego:"clear"`
	ClearColor mathx.Color    `aego:"clearColor"`
	Scale      view.ScaleMode `aego:"scale"`
	Virtual    mathx.Vec2     `aego:"virtual"`
	Active     bool           `aego:"active"`
}

func DefaultSpriteRenderer() SpriteRenderer {
	return SpriteRenderer{
		Tint:   mathx.Color{R: 1, G: 1, B: 1, A: 1},
		Origin: mathx.Vec2{X: 0.5, Y: 0.5},
	}
}

func DefaultCamera2D() Camera2D {
	return Camera2D{
		Zoom:       64,
		Viewport:   mathx.Rect{W: 1, H: 1},
		Mask:       -1,
		Clear:      true,
		ClearColor: mathx.Color{R: 0.04, G: 0.05, B: 0.07, A: 1},
		Active:     true,
	}
}

func Rotation(m mathx.Mat3) float32 {
	return mathx.Atan2(m[1], m[0])
}

func ScaleX(m mathx.Mat3) float32 {
	return mathx.Sqrt(m[0]*m[0] + m[1]*m[1])
}

func ScaleY(m mathx.Mat3) float32 {
	return mathx.Sqrt(m[3]*m[3] + m[4]*m[4])
}

func Size(sr *SpriteRenderer, region mathx.Vec2) mathx.Vec2 {
	if sr.Size.X == 0 && sr.Size.Y == 0 {
		return region
	}
	return sr.Size
}
