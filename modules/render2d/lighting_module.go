package render2d

import (
	"aego/engine/module"
	"aego/render/lighting"
	"aego/scene"
	"aego/schema"
)

func registerLighting(r *module.Registry) error {
	if r.Bindings() != nil {
		installLightingScript(r.ScriptModule("aego/lighting"))
	}
	if err := module.Enum[lighting.Type](r, "engine.Light2DType",
		schema.EnumValue{Name: "point", Value: int64(lighting.Point)}, schema.EnumValue{Name: "spot", Value: int64(lighting.Spot)},
		schema.EnumValue{Name: "directional", Value: int64(lighting.Directional)}, schema.EnumValue{Name: "sprite", Value: int64(lighting.Sprite)}); err != nil {
		return err
	}
	if err := module.Enum[OccluderSource](r, "engine.LightOccluderSource",
		schema.EnumValue{Name: "polygon", Value: int64(OccluderPolygon)}, schema.EnumValue{Name: "shapeProvider", Value: int64(OccluderShapeProvider)}, schema.EnumValue{Name: "spriteAlpha", Value: int64(OccluderSpriteAlpha)}); err != nil {
		return err
	}
	if err := module.Enum[lighting.CullMode](r, "engine.ShadowCullMode",
		schema.EnumValue{Name: "none", Value: int64(lighting.CullNone)}, schema.EnumValue{Name: "clockwise", Value: int64(lighting.CullClockwise)},
		schema.EnumValue{Name: "counterClockwise", Value: int64(lighting.CullCounterClockwise)}); err != nil {
		return err
	}
	if err := module.Enum[lighting.ColorMode](r, "engine.LightingColorMode",
		schema.EnumValue{Name: "auto", Value: int64(lighting.Auto)}, schema.EnumValue{Name: "ldr", Value: int64(lighting.LDR)},
		schema.EnumValue{Name: "hdr", Value: int64(lighting.HDR)}); err != nil {
		return err
	}
	if err := module.Component[Light2D](r, schema.Name("engine.Light2D"), schema.Space(schema.Space2D),
		schema.Doc("Forward 2D diffuse light. Local +X defines Spot and Directional azimuth; presentation transforms also drive shadows."),
		schema.FieldDoc("type", "Point, Spot, Directional or Sprite/cookie. Directional shadows are not supported."),
		schema.FieldDoc("color", "Linear light color; the color picker handles display conversion."),
		schema.FieldDoc("intensity", "Nonnegative energy multiplier; zero disables this light's contribution."),
		schema.FieldDoc("range", "Local light radius in world units, independent of node scale."),
		schema.FieldDoc("height", "Virtual distance above the sprite surface for diffuse normal mapping; shadows remain planar."),
		schema.FieldDoc("innerAngle", "Full Spot inner cone angle in radians, centered on local +X."),
		schema.FieldDoc("outerAngle", "Full Spot outer cone angle in radians, no smaller than innerAngle."),
		schema.FieldDoc("elevation", "Directional elevation above the surface in radians, greater than zero."),
		schema.FieldDoc("falloff", "Nonnegative falloff exponent over normalized local-light distance."),
		schema.FieldDoc("cookie", "Sprite light mask texture. Its own region and trim map to the light square."),
		schema.FieldDoc("layers", "64-bit receiver visibility mask. -1 means all bits; zero receives no objects."),
		schema.FieldDoc("shadowCasterMask", "64-bit shadow-occluder mask, independent of visibility and collision masks."),
		schema.FieldDoc("shadows", "Enable cached local-light shadow masks for Point, Spot and Sprite lights."),
		schema.FieldDoc("shadowSoftness", "Bounded mask filtering radius; this is not a physical area-light penumbra."),
		schema.FieldDoc("priority", "Higher priority is selected first when the per-draw light budget is exceeded."),
	).Category("Lighting").Display("Light 2D").Icon("comp.sprite").NodeIcon(55).Default(DefaultLight2D).Validate(validateLight2D).Err(); err != nil {
		return err
	}
	if err := module.Component[LightOccluder2D](r, schema.Name("engine.LightOccluder2D"), schema.Space(schema.Space2D),
		schema.Doc("A planar shadow contour. Manual polygons require no Physics module; ShapeProvider reads local geometry through the shapes2d contract."),
		schema.FieldDoc("source", "Use a manual polygon/chain or a registered local-shape provider."),
		schema.FieldDoc("polygon", "Simple local-space contour without holes; points follow the owner's presentation transform once."),
		schema.FieldDoc("closed", "Close the manual polygon. A ShapeProvider supplies each contour's closed state."),
		schema.FieldDoc("cullMode", "Shadow edge winding policy; independent of any physics one-sided setting."),
		schema.FieldDoc("layers", "64-bit shadow participation mask. -1 is all bits; zero casts no shadows."),
		schema.FieldDoc("shapeNode", "Source node for ShapeProvider. Empty uses this node; unresolved IDs are preserved."),
		schema.FieldDoc("shape", "Stable provider shape key, such as physics.BoxCollider2D. Empty selects all of the source node's shapes."),
	).Category("Lighting").Display("Light Occluder 2D").Icon("comp.mesh").Default(DefaultLightOccluder2D).
		Validate(validateLightOccluder2D).Template("Light Occluder", nil).Err(); err != nil {
		return err
	}
	if err := module.Component[LightingEnvironment2D](r, schema.Name("engine.LightingEnvironment2D"), schema.Space(schema.Space2D),
		schema.Doc("Lighting and output policy for a camera. An empty camera applies to the current output; overlapping environments are an error."),
		schema.FieldDoc("camera", "Optional Camera2D node. Empty applies to all 2D views of this world."),
		schema.FieldDoc("ambientColor", "Linear ambient surface illumination. Default white preserves unlit-looking Lit scenes with no lights."),
		schema.FieldDoc("ambientIntensity", "Nonnegative ambient energy multiplier."),
		schema.FieldDoc("colorMode", "Auto chooses supported HDR for Lit output; LDR uses RGBA8 and HDR requests RGBA16F."),
		schema.FieldDoc("exposure", "Positive display-resolve exposure, without changing UI or simulation."),
		schema.FieldDoc("shadowResolution", "Power-of-two mask dimension between 64 and 2048. Initial profile uses 256."),
		schema.FieldDoc("maxShadowedLights", "Per-camera local-shadow budget from 0 through 16; actual performance depends on the GPU."),
	).Category("Lighting").Display("Lighting Environment 2D").Icon("comp.camera2d").Default(DefaultLightingEnvironment2D).
		Validate(validateLightingEnvironment2D).Template("Lighting Environment", nil).Err(); err != nil {
		return err
	}
	for _, t := range []struct {
		id, name string
		kind     lighting.Type
	}{
		{"point-light", "Point Light", lighting.Point}, {"spot-light", "Spot Light", lighting.Spot},
		{"directional-light", "Directional Light", lighting.Directional}, {"sprite-light", "Sprite Light", lighting.Sprite},
	} {
		kind := t.kind
		r.Template(schema.TemplateDesc{ID: ID + "." + t.id, Name: t.name, Menu: "Lighting", Icon: "comp.sprite", Space: schema.Space2D,
			Setup: func(s *scene.Scene, h scene.NodeHandle) {
				l := DefaultLight2D()
				l.Type = kind
				*scene.Add[Light2D](s, h) = l
			}})
	}
	return nil
}
