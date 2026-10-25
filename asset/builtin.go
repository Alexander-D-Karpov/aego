package asset

import (
	"aego/core/ids"
	"aego/render/mesh"
)

const builtinHi uint64 = 0xae90000000000000

var (
	BuiltinCube     = ids.AssetID{Hi: builtinHi, Lo: 1}
	BuiltinPlane    = ids.AssetID{Hi: builtinHi, Lo: 2}
	BuiltinSphere   = ids.AssetID{Hi: builtinHi, Lo: 3}
	BuiltinCylinder = ids.AssetID{Hi: builtinHi, Lo: 4}
	BuiltinWhite    = ids.AssetID{Hi: builtinHi, Lo: 16}
)

type Builtin struct {
	ID   ids.AssetID
	Name string
	Kind ArtifactKind
}

var builtins = [...]Builtin{
	{ID: BuiltinCube, Name: "engine/meshes/cube", Kind: ArtMesh},
	{ID: BuiltinPlane, Name: "engine/meshes/plane", Kind: ArtMesh},
	{ID: BuiltinSphere, Name: "engine/meshes/sphere", Kind: ArtMesh},
	{ID: BuiltinCylinder, Name: "engine/meshes/cylinder", Kind: ArtMesh},
	{ID: BuiltinWhite, Name: "engine/textures/white", Kind: ArtTexture},
}

var allBuiltins = append(append(builtins[:len(builtins):len(builtins)], builtinAudioEntries()...), builtinAnimEntries()...)

func Builtins() []Builtin {
	return allBuiltins
}

func IsBuiltin(id ids.AssetID) bool {
	return id.Hi == builtinHi
}

func LookupBuiltin(id ids.AssetID) (Builtin, bool) {
	for _, b := range allBuiltins {
		if b.ID == id {
			return b, true
		}
	}
	return Builtin{}, false
}

func BuiltinMesh(id ids.AssetID) (*mesh.Data, bool) {
	switch id {
	case BuiltinCube:
		return mesh.Cube(1), true
	case BuiltinPlane:
		return mesh.Plane(10, 10, 10), true
	case BuiltinSphere:
		return mesh.Sphere(0.5, 16, 24), true
	case BuiltinCylinder:
		return mesh.Cylinder(0.5, 1, 24), true
	}
	return nil, false
}
