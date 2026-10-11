package render2d

import (
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
)

const (
	ID            = "render2d"
	SystemCameras = ID + ".cameras"
	SystemExtract = ID + ".extract"
)

type Module struct{}

func New() Module {
	return Module{}
}

func (Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true}
}

func (Module) Register(r *module.Registry) error {
	if err := module.Enum[view.ScaleMode](r, "engine.ScaleMode",
		schema.EnumValue{Name: "free", Value: int64(view.ScaleFree)},
		schema.EnumValue{Name: "pixelPerfect", Value: int64(view.ScalePixelPerfect)},
		schema.EnumValue{Name: "fitVirtual", Value: int64(view.ScaleFitVirtual)},
	); err != nil {
		return err
	}
	if err := module.Component[SpriteRenderer](r,
		schema.Name("engine.SpriteRenderer"),
		schema.Space(schema.Space2D),
		schema.Doc("Draws a texture region at the node transform."),
		schema.FieldDoc("sprite", "Texture or atlas region to draw. Empty draws a solid rectangle in the tint color."),
		schema.FieldDoc("layer", "Sorting layer. Higher layers are drawn on top of lower ones."),
		schema.FieldDoc("order", "Order inside the layer. Higher values are drawn later, on top."),
		schema.FieldDoc("tint", "Color multiplied with the texture. Alpha controls transparency."),
		schema.FieldDoc("origin", "Pivot in normalized sprite space. (0.5, 0.5) is the center, (0, 0) the top-left corner."),
		schema.FieldDoc("size", "Size in world units. Zero uses the texture size divided by the camera zoom."),
		schema.FieldDoc("flipX", "Mirrors the sprite horizontally."),
		schema.FieldDoc("flipY", "Mirrors the sprite vertically."),
		schema.FieldDoc("blend", "How the sprite blends with what is behind it: premultiplied alpha, opaque or additive."),
		schema.FieldDoc("filter", "Texture sampling: nearest keeps pixel art sharp, linear smooths it."),
	).
		Category("Rendering").
		Display("Sprite Renderer").
		Icon("cat.rendering").
		Default(DefaultSpriteRenderer).
		Template("Sprite", unitSprite).
		Err(); err != nil {
		return err
	}
	if err := module.Component[Camera2D](r,
		schema.Name("engine.Camera2D"),
		schema.FormerName("engine.Camera"),
		schema.Space(schema.Space2D),
		schema.Doc("Renders an orthographic view from the node transform."),
		schema.FieldDoc("zoom", "Pixels per world unit. Larger values show a smaller part of the world."),
		schema.FieldDoc("viewport", "Part of the screen the camera draws into, in normalized coordinates."),
		schema.FieldDoc("mask", "Visibility layers this camera renders. -1 renders every layer."),
		schema.FieldDoc("order", "Cameras with a lower order render first."),
		schema.FieldDoc("clear", "Clears the viewport before drawing."),
		schema.FieldDoc("clearColor", "Background color used when clear is on."),
		schema.FieldDoc("scale", "How the view adapts to the window: free, pixelPerfect or fitVirtual."),
		schema.FieldDoc("virtual", "Virtual resolution used by fitVirtual and pixelPerfect."),
		schema.FieldDoc("active", "Only active cameras render."),
	).
		Category("Camera").
		Display("Camera 2D").
		Icon("cat.camera").
		Default(DefaultCamera2D).
		Template("Camera", nil).
		Err(); err != nil {
		return err
	}
	r.System(engine.SystemDesc{
		ID:     SystemCameras,
		Phase:  engine.PhaseRenderPrepare,
		Scope:  engine.ScopeWorld,
		After:  []string{frame.SystemTransform},
		System: cameras{},
	})
	r.System(engine.SystemDesc{
		ID:     SystemExtract,
		Phase:  engine.PhaseRender,
		Scope:  engine.ScopeWorld,
		Before: []string{frame.SystemSubmit},
		System: extract{},
	})
	return nil
}

func unitSprite(s *scene.Scene, h scene.NodeHandle) {
	if sr, ok := scene.Get[SpriteRenderer](s, h); ok {
		sr.Size = mathx.Vec2{X: 1, Y: 1}
	}
}
