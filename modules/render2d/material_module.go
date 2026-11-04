package render2d

import (
	"math"

	"aego/asset"
	"aego/scene"

	"aego/core/mathx"
	"aego/engine/module"
	"aego/render/shader"
	"aego/schema"
	"aego/script/bind"
)

func registerMaterials(r *module.Registry) error {
	if r.Bindings() != nil {
		m := r.ScriptModule("aego/material")
		bind.Struct[mathx.Vec4](m, "Vec4", bind.Doc("Four numeric lanes, with no color-space conversion."))
		bind.Func1(m, "Builtin", func(c *bind.Ctx, name string) asset.Ref[asset.Material] {
			id, ok := asset.BuiltinMaterialNamed(name)
			if !ok {
				c.Fail("unknown built-in material " + name)
			}
			return asset.Ref[asset.Material]{ID: id}
		}, bind.Doc("Builtin returns Unlit (Default), Additive, Flash, Dissolve, Outline or PixelSnap. Lit belongs to the lighting module."), bind.Params("name"))
		bind.Func1(m, "Shader", func(c *bind.Ctx, name string) asset.Ref[asset.Shader] {
			id, ok := asset.BuiltinShaderNamed(name)
			if !ok {
				c.Fail("unknown built-in shader " + name)
			}
			return asset.Ref[asset.Shader]{ID: id}
		}, bind.Doc("Shader returns the prepared shader reference for a named built-in material."), bind.Params("name"))
		bind.Proc2(m, "Assign", func(c *bind.Ctx, node scene.NodeHandle, ref asset.Ref[asset.Material]) {
			c.FailErr(AssignMaterial(c.Scene, node, ref.ID))
		}, bind.Doc("Assign changes the SpriteRenderer or Mesh2D material reference in this world."), bind.Params("node", "material"), bind.Example(`material.Assign(node, material.Builtin("Flash"))`))
	}

	if err := module.Enum[shader.Kind](r, "engine.MaterialParameterKind",
		schema.EnumValue{Name: "float", Value: int64(shader.KindFloat)},
		schema.EnumValue{Name: "int", Value: int64(shader.KindInt)},
		schema.EnumValue{Name: "bool", Value: int64(shader.KindBool)},
		schema.EnumValue{Name: "vec2", Value: int64(shader.KindVec2)},
		schema.EnumValue{Name: "vec3", Value: int64(shader.KindVec3)},
		schema.EnumValue{Name: "vec4", Value: int64(shader.KindVec4)},
		schema.EnumValue{Name: "texture", Value: int64(shader.KindTexture)},
	); err != nil {
		return err
	}
	if err := module.Struct[MaterialParameter](r, schema.Name("engine.MaterialParameter")); err != nil {
		return err
	}
	if err := module.Component[Mesh2D](r, schema.Name("engine.Mesh2D"), schema.Space(schema.Space2D),
		schema.Doc("Draws the standard mesh vertex layout in a 2D camera with a compatible Mesh2D material."),
		schema.FieldDoc("material", "Shared material asset. The shader domain must be mesh2d."),
		schema.FieldDoc("mesh", "Mesh asset with the standard position, normal, UV and vertex color layout."),
		schema.FieldDoc("texture", "Main texture sampled in mesh UV coordinates. Empty uses white."),
		schema.FieldDoc("tint", "Linear color multiplied with the texture; alpha controls transparency."),
		schema.FieldDoc("layer", "Visual layer for this 2D mesh."),
		schema.FieldDoc("order", "Painter order among sprites and meshes in the same layer; higher values draw later."),
	).Category("Rendering").Display("Mesh 2D").Icon("comp.mesh").Default(DefaultMesh2D).Validate(validateMeshMaterial).Template("Mesh 2D", nil).Err(); err != nil {
		return err
	}
	b := module.Component[MaterialOverride](r, schema.Name("engine.MaterialOverride"), schema.Space(schema.Space2D),
		schema.Doc("Overrides up to eight numeric shader parameters on this node. Removed parameters remain in the values list."),
		schema.FieldDoc("values", "Explicit typed values. Absent values inherit the shared material; removed shader fields remain serialized."),
	).Category("Rendering").Display("Material Override").Icon("comp.sprite").RequiresOneOf("engine.SpriteRenderer", "engine.Mesh2D").Validate(validateOverrides).ScriptNone()
	b.Proc2("SetFloat", func(c *bind.Ctx, _ *MaterialOverride, name string, v float32) {
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Float32(v)))
	}, bind.Doc("Sets an instance float parameter on this node. Type mismatch leaves its previous value unchanged."))
	b.Proc2("SetInt", func(c *bind.Ctx, _ *MaterialOverride, name string, v int64) {
		if v < math.MinInt32 || v > math.MaxInt32 {
			c.Fail("material integer is outside int32 range")
			return
		}
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Int32(int32(v))))
	}, bind.Doc("Sets an exact signed int32 instance parameter. Values outside the int32 range are rejected."))
	b.Proc2("SetBool", func(c *bind.Ctx, _ *MaterialOverride, name string, v bool) {
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Bool(v)))
	}, bind.Doc("Sets an instance bool parameter on this node."))
	b.Proc2("SetVec2", func(c *bind.Ctx, _ *MaterialOverride, name string, v mathx.Vec2) {
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Vec2(v)))
	}, bind.Doc("Sets an instance vec2 parameter on this node."))
	b.Proc2("SetVec3", func(c *bind.Ctx, _ *MaterialOverride, name string, v mathx.Vec3) {
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Vec3(v)))
	}, bind.Doc("Sets an instance vec3 parameter on this node."))
	b.Proc2("SetVec4", func(c *bind.Ctx, _ *MaterialOverride, name string, v mathx.Vec4) {
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Vec4(v)))
	}, bind.Doc("Sets four numerical instance lanes using material.Vec4 without color conversion."))
	b.Proc2("SetColor", func(c *bind.Ctx, _ *MaterialOverride, name string, v mathx.Color) {
		c.FailErr(SetMaterialParameter(c.Scene, c.Recv, name, schema.Color(v)))
	}, bind.Doc("Sets a linear Color instance parameter declared with the color hint."))
	b.Method1("Reset", func(c *bind.Ctx, _ *MaterialOverride, name string) bool {
		return ResetMaterialParameter(c.Scene, c.Recv, name)
	}, bind.Doc("Removes the explicit instance value and inherits the shared material value. Returns whether an explicit value existed."))
	b.Method1("GetFloat", func(c *bind.Ctx, _ *MaterialOverride, name string) float32 {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindFloat32 {
			c.Fail("material parameter is not float")
		}
		return float32(v.F)
	}, bind.Doc("Reads the effective float after shader default, material value and node override."))
	b.Method1("GetInt", func(c *bind.Ctx, _ *MaterialOverride, name string) int64 {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindInt32 {
			c.Fail("material parameter is not int")
		}
		return v.I
	}, bind.Doc("Reads the exact signed int32 effective parameter."))
	b.Method1("GetBool", func(c *bind.Ctx, _ *MaterialOverride, name string) bool {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindBool {
			c.Fail("material parameter is not bool")
		}
		return v.I != 0
	}, bind.Doc("Reads the effective bool parameter."))
	b.Method1("GetVec2", func(c *bind.Ctx, _ *MaterialOverride, name string) mathx.Vec2 {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindVec2 {
			c.Fail("material parameter is not vec2")
		}
		return v.V
	}, bind.Doc("Reads the effective vec2 parameter."))
	b.Method1("GetVec3", func(c *bind.Ctx, _ *MaterialOverride, name string) mathx.Vec3 {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindVec3 {
			c.Fail("material parameter is not vec3")
		}
		return v.V3
	}, bind.Doc("Reads the effective vec3 parameter."))
	b.Method1("GetVec4", func(c *bind.Ctx, _ *MaterialOverride, name string) mathx.Vec4 {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindVec4 {
			c.Fail("material parameter is not vec4")
		}
		return v.V4
	}, bind.Doc("Reads four numerical effective lanes as material.Vec4, without color conversion."))
	b.Method2("GetVec4Lane", func(c *bind.Ctx, _ *MaterialOverride, name string, lane int64) float32 {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err != nil {
			return 0
		}
		if v.Kind != schema.KindVec4 || lane < 0 || lane > 3 {
			c.Fail("GetVec4Lane needs a vec4 parameter and a lane from 0 through 3")
			return 0
		}
		return [4]float32{v.V4.X, v.V4.Y, v.V4.Z, v.V4.W}[lane]
	}, bind.Doc("Reads one lane from an ordinary vec4 parameter. The lane index must be from 0 through 3."))
	b.Method1("GetColor", func(c *bind.Ctx, _ *MaterialOverride, name string) mathx.Color {
		v, err := GetMaterialParameter(c.Scene, c.Recv, name)
		c.FailErr(err)
		if err == nil && v.Kind != schema.KindColor {
			c.Fail("material parameter is not color")
		}
		return v.C
	}, bind.Doc("Reads the effective linear Color of a parameter declared with the color hint."))
	if err := b.Err(); err != nil {
		return err
	}
	return r.Types().RegisterBoundProperties("engine.MaterialOverride", resolveMaterialProperty)
}
