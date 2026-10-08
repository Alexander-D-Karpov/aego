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

type Camera3D struct {
	FOV        float32     `aego:"fov,min=1,max=179,unit=deg"`
	Near       float32     `aego:"near,min=0.001"`
	Far        float32     `aego:"far,min=0.01"`
	Viewport   mathx.Rect  `aego:"viewport"`
	Mask       int64       `aego:"mask"`
	Order      int32       `aego:"order"`
	Clear      bool        `aego:"clear"`
	ClearColor mathx.Color `aego:"clearColor"`
	Active     bool        `aego:"active"`
}

type MeshRenderer struct {
	Mesh        asset.Ref[asset.Mesh]    `aego:"mesh,asset=mesh"`
	Texture     asset.Ref[asset.Texture] `aego:"texture,asset=texture"`
	Color       mathx.Color              `aego:"color"`
	Blend       sprite.BlendMode         `aego:"blend"`
	DoubleSided bool                     `aego:"doubleSided"`
	Layer       int32                    `aego:"layer"`
	Filter      gpu.FilterMode           `aego:"filter"`
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

func DefaultCamera3D() Camera3D {
	return Camera3D{
		FOV:        60,
		Near:       0.1,
		Far:        1000,
		Viewport:   mathx.Rect{W: 1, H: 1},
		Mask:       -1,
		Clear:      true,
		ClearColor: mathx.Color{R: 0.04, G: 0.05, B: 0.07, A: 1},
		Active:     true,
	}
}

func DefaultMeshRenderer() MeshRenderer {
	return MeshRenderer{
		Color:  mathx.Color{R: 1, G: 1, B: 1, A: 1},
		Blend:  sprite.BlendOpaque,
		Filter: gpu.FilterLinear,
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
		schema.Space(schema.Space2D),
		schema.Doc("draws a texture region at the node transform")); err != nil {
		return err
	}
	if err := scene.RegisterComponent[Camera2D](t,
		schema.Name("engine.Camera2D"),
		schema.FormerName("engine.Camera"),
		schema.Space(schema.Space2D),
		schema.Doc("renders an orthographic view from the node transform")); err != nil {
		return err
	}
	if err := scene.RegisterComponent[Camera3D](t,
		schema.Name("engine.Camera3D"),
		schema.Space(schema.Space3D),
		schema.Doc("renders a perspective view looking down the node -Z axis")); err != nil {
		return err
	}
	if err := scene.RegisterComponent[MeshRenderer](t,
		schema.Name("engine.MeshRenderer"),
		schema.Space(schema.Space3D),
		schema.Doc("draws an unlit mesh at the node transform")); err != nil {
		return err
	}
	return registerScripts(t)
}
