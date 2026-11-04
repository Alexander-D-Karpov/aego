package sdk

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/modules/render2d"
	"aego/modules/render3d"
	"aego/scene"
	"aego/schema"
)

type (
	Vec3              = mathx.Vec3
	Vec4              = mathx.Vec4
	Quat              = mathx.Quat
	Mat4              = mathx.Mat4
	Transform3D       = scene.Transform3D
	Space             = scene.Space
	SpaceKind         = schema.SpaceKind
	SpriteRenderer    = render2d.SpriteRenderer
	MaterialOverride  = render2d.MaterialOverride
	MaterialParameter = render2d.MaterialParameter
	Mesh2D            = render2d.Mesh2D
	MaterialRef       = asset.Ref[asset.Material]
	ShaderRef         = asset.Ref[asset.Shader]
	Camera2D          = render2d.Camera2D
	Camera3D          = render3d.Camera3D
	MeshRenderer      = render3d.MeshRenderer
	MeshRef           = asset.Ref[asset.Mesh]
)

const (
	Space2D  = scene.Space2D
	Space3D  = scene.Space3D
	SpaceAny = schema.SpaceAny
	Kind2D   = schema.Space2D
	Kind3D   = schema.Space3D
)

var (
	QuatIdentity             = mathx.QuatIdentity
	QuatAxisAngle            = mathx.QuatAxisAngle
	QuatEuler                = mathx.QuatEuler
	QuatLookDir              = mathx.QuatLookDir
	IdentityTransform3D      = scene.IdentityTransform3D
	DefaultSpriteRenderer    = render2d.DefaultSpriteRenderer
	DefaultMesh2D            = render2d.DefaultMesh2D
	AssignMaterial           = render2d.AssignMaterial
	SetMaterialParameter     = render2d.SetMaterialParameter
	GetMaterialParameter     = render2d.GetMaterialParameter
	ResetMaterialParameter   = render2d.ResetMaterialParameter
	DefaultCamera2D          = render2d.DefaultCamera2D
	DefaultCamera3D          = render3d.DefaultCamera3D
	DefaultMeshRenderer      = render3d.DefaultMeshRenderer
	BuiltinMaterialNamed     = asset.BuiltinMaterialNamed
	BuiltinShaderNamed       = asset.BuiltinShaderNamed
	BuiltinMaterialUnlit     = asset.BuiltinMaterialUnlit
	BuiltinMaterialAdditive  = asset.BuiltinMaterialAdditive
	BuiltinMaterialFlash     = asset.BuiltinMaterialFlash
	BuiltinMaterialDissolve  = asset.BuiltinMaterialDissolve
	BuiltinMaterialOutline   = asset.BuiltinMaterialOutline
	BuiltinMaterialPixelSnap = asset.BuiltinMaterialPixelSnap
	BuiltinCube              = asset.BuiltinCube
	BuiltinPlane             = asset.BuiltinPlane
	BuiltinSphere            = asset.BuiltinSphere
	BuiltinCylinder          = asset.BuiltinCylinder
)
