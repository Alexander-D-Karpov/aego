package components

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
	"aego/schema"
)

type SpriteRenderer struct {
	Sprite asset.Ref[asset.Texture] `aego:"sprite,asset=texture"`
	Layer  int32                    `aego:"layer"`
	Order  int32                    `aego:"order"`
	Tint   mathx.Color              `aego:"tint"`
	Origin mathx.Vec2               `aego:"origin"`
	Size   mathx.Vec2               `aego:"size"`
	FlipX  bool                     `aego:"flipX"`
	FlipY  bool                     `aego:"flipY"`
	Blend  sprite.BlendMode         `aego:"blend"`
	Filter gpu.FilterMode           `aego:"filter"`
}

type Camera struct {
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

func DefaultCamera() Camera {
	return Camera{
		Zoom:       64,
		Viewport:   mathx.Rect{W: 1, H: 1},
		Mask:       -1,
		Clear:      true,
		ClearColor: mathx.Color{R: 0.04, G: 0.05, B: 0.07, A: 1},
		Active:     true,
	}
}

func Register(t *scene.Types) error {
	r := t.Schema()
	if err := schema.RegisterEnum[sprite.BlendMode](r, "engine.BlendMode",
		schema.EnumValue{Name: "premultiplied", Value: int64(sprite.BlendPremultiplied)},
		schema.EnumValue{Name: "opaque", Value: int64(sprite.BlendOpaque)},
		schema.EnumValue{Name: "additive", Value: int64(sprite.BlendAdditive)},
	); err != nil {
		return err
	}
	if err := schema.RegisterEnum[gpu.FilterMode](r, "engine.FilterMode",
		schema.EnumValue{Name: "nearest", Value: int64(gpu.FilterNearest)},
		schema.EnumValue{Name: "linear", Value: int64(gpu.FilterLinear)},
	); err != nil {
		return err
	}
	if err := schema.RegisterEnum[view.ScaleMode](r, "engine.ScaleMode",
		schema.EnumValue{Name: "free", Value: int64(view.ScaleFree)},
		schema.EnumValue{Name: "pixelPerfect", Value: int64(view.ScalePixelPerfect)},
		schema.EnumValue{Name: "fitVirtual", Value: int64(view.ScaleFitVirtual)},
	); err != nil {
		return err
	}
	if err := scene.RegisterComponent[SpriteRenderer](t,
		schema.Name("engine.SpriteRenderer"),
		schema.Doc("draws a texture region at the node transform")); err != nil {
		return err
	}
	return scene.RegisterComponent[Camera](t,
		schema.Name("engine.Camera"),
		schema.Doc("renders a view from the node transform"))
}
