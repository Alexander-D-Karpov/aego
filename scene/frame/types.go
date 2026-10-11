package frame

import (
	"aego/gpu"
	"aego/render/sprite"
	"aego/scene"
	"aego/schema"
)

func RegisterTypes(t *scene.Types) error {
	r := t.Schema()
	if err := schema.RegisterEnum[sprite.BlendMode](r, "engine.BlendMode",
		schema.EnumValue{Name: "premultiplied", Value: int64(sprite.BlendPremultiplied)},
		schema.EnumValue{Name: "opaque", Value: int64(sprite.BlendOpaque)},
		schema.EnumValue{Name: "additive", Value: int64(sprite.BlendAdditive)},
	); err != nil {
		return err
	}
	return schema.RegisterEnum[gpu.FilterMode](r, "engine.FilterMode",
		schema.EnumValue{Name: "nearest", Value: int64(gpu.FilterNearest)},
		schema.EnumValue{Name: "linear", Value: int64(gpu.FilterLinear)},
	)
}
