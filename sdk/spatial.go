package sdk

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/scene"
	"aego/scene/components"
)

type (
	Vec3         = mathx.Vec3
	Vec4         = mathx.Vec4
	Quat         = mathx.Quat
	Mat4         = mathx.Mat4
	Transform3D  = scene.Transform3D
	Space        = scene.Space
	Camera2D     = components.Camera2D
	Camera3D     = components.Camera3D
	MeshRenderer = components.MeshRenderer
	MeshRef      = asset.Ref[asset.Mesh]
)

const (
	Space2D = scene.Space2D
	Space3D = scene.Space3D
)

var (
	QuatIdentity        = mathx.QuatIdentity
	QuatAxisAngle       = mathx.QuatAxisAngle
	QuatEuler           = mathx.QuatEuler
	QuatLookDir         = mathx.QuatLookDir
	IdentityTransform3D = scene.IdentityTransform3D
	BuiltinCube         = asset.BuiltinCube
	BuiltinPlane        = asset.BuiltinPlane
	BuiltinSphere       = asset.BuiltinSphere
	BuiltinCylinder     = asset.BuiltinCylinder
)
