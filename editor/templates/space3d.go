package templates

import (
	"strings"

	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
)

const Category3D = "3D"

type Shape3D struct {
	Name string
	Mesh ids.AssetID
}

var Shapes3D = []Shape3D{
	{Name: "Cube", Mesh: asset.BuiltinCube},
	{Name: "Plane", Mesh: asset.BuiltinPlane},
	{Name: "Sphere", Mesh: asset.BuiltinSphere},
	{Name: "Cylinder", Mesh: asset.BuiltinCylinder},
}

const Moved3DNote = "a 3D node needs a 3D parent; created at the scene root"

func Parent3D(s *scene.Scene, parent scene.NodeHandle) (scene.NodeHandle, string) {
	if parent.IsZero() || !s.IsAlive(parent) {
		return s.Root(), ""
	}
	if s.AllowsSpace(parent, scene.Space3D) {
		return parent, ""
	}
	return s.Root(), Moved3DNote
}

func BuildEmpty3D(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
	return s.Create3D(parent, "Empty")
}

func BuildCamera3D(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
	h := s.Create3D(parent, "Camera")
	if h.IsZero() {
		return h
	}
	*scene.Add[components.Camera3D](s, h) = components.DefaultCamera3D()
	pos := mathx.Vec3{Y: 2, Z: 6}
	s.SetLocal3D(h, scene.Transform3D{Position: pos, Rotation: mathx.QuatLookDir(pos.Neg()), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}})
	return h
}

func BuildShape3D(s *scene.Scene, parent scene.NodeHandle, shape Shape3D) scene.NodeHandle {
	h := s.Create3D(parent, shape.Name)
	if h.IsZero() {
		return h
	}
	mr := scene.Add[components.MeshRenderer](s, h)
	*mr = components.DefaultMeshRenderer()
	mr.Mesh.SetAssetRefID(shape.Mesh)
	return h
}

func Builtin3D() []Template {
	out := []Template{
		{ID: "engine.empty3d", Name: "Empty", Module: "engine", Category: Category3D, Space: scene.Space3D, Build: BuildEmpty3D},
		{ID: "engine.camera3d", Name: "Camera", Module: "engine", Category: Category3D, Space: scene.Space3D, Build: BuildCamera3D},
	}
	for _, sh := range Shapes3D {
		out = append(out, Template{
			ID:       "engine." + strings.ToLower(sh.Name),
			Name:     sh.Name,
			Module:   "engine",
			Category: Category3D,
			Space:    scene.Space3D,
			Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
				return BuildShape3D(s, parent, sh)
			},
		})
	}
	return out
}

func MeshFor(id ids.AssetID, kind asset.ArtifactKind, name string) Template {
	meshID, texID := id, ids.AssetID{}
	if kind != asset.ArtMesh {
		meshID, texID = asset.BuiltinPlane, id
	}
	if name == "" {
		name = "Mesh"
	}
	return Template{ID: "engine.mesh", Name: name, Module: "engine", Category: Category3D, Space: scene.Space3D,
		Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
			h := s.Create3D(parent, name)
			if h.IsZero() {
				return h
			}
			mr := scene.Add[components.MeshRenderer](s, h)
			*mr = components.DefaultMeshRenderer()
			mr.Mesh.SetAssetRefID(meshID)
			mr.Texture.SetAssetRefID(texID)
			return h
		}}
}

func Prefab3D(types *scene.Types, build func(*scene.Scene, scene.NodeHandle) scene.NodeHandle) []byte {
	s := scene.New(types, nil, scene.Options{DefaultSpace: scene.Space3D})
	h := build(s, s.Root())
	s.Flush()
	return sceneio.SaveSubtree(s, h, true)
}
