package render3d

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/gpu"
	"aego/render/sprite"
)

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

type Shape struct {
	Name string
	Mesh ids.AssetID
}

var Shapes = []Shape{
	{Name: "Cube", Mesh: asset.BuiltinCube},
	{Name: "Plane", Mesh: asset.BuiltinPlane},
	{Name: "Sphere", Mesh: asset.BuiltinSphere},
	{Name: "Cylinder", Mesh: asset.BuiltinCylinder},
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
