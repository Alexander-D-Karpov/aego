package render3d

import (
	"strings"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/scene"
	"aego/scene/frame"
	"aego/schema"
)

const (
	ID            = "render3d"
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
	if err := module.Component[Camera3D](r,
		schema.Name("engine.Camera3D"),
		schema.Space(schema.Space3D),
		schema.Doc("Renders a perspective view looking down the node -Z axis."),
		schema.FieldDoc("fov", "Vertical field of view in degrees."),
		schema.FieldDoc("near", "Distance to the near clipping plane."),
		schema.FieldDoc("far", "Distance to the far clipping plane."),
		schema.FieldDoc("viewport", "Part of the screen the camera draws into, in normalized coordinates."),
		schema.FieldDoc("mask", "Visibility layers this camera renders. -1 renders every layer."),
		schema.FieldDoc("order", "Cameras with a lower order render first."),
		schema.FieldDoc("clear", "Clears the viewport before drawing."),
		schema.FieldDoc("clearColor", "Background color used when clear is on."),
		schema.FieldDoc("active", "Only active cameras render."),
	).
		Category("Camera").
		Display("Camera 3D").
		Icon("cat.camera").
		Default(DefaultCamera3D).
		Template("Camera", lookAtOrigin).
		Err(); err != nil {
		return err
	}
	if err := module.Component[MeshRenderer](r,
		schema.Name("engine.MeshRenderer"),
		schema.Space(schema.Space3D),
		schema.Doc("Draws an unlit mesh at the node transform."),
		schema.FieldDoc("mesh", "Mesh to draw: an imported .obj or a builtin Cube, Plane, Sphere or Cylinder."),
		schema.FieldDoc("texture", "Texture mapped onto the mesh. Empty uses white."),
		schema.FieldDoc("color", "Color multiplied with the texture."),
		schema.FieldDoc("blend", "Opaque writes depth. Premultiplied and additive are drawn after opaque meshes."),
		schema.FieldDoc("doubleSided", "Draws the back faces too."),
		schema.FieldDoc("layer", "Sorting layer for transparent meshes."),
		schema.FieldDoc("filter", "Texture sampling: nearest or linear."),
	).
		Category("Rendering").
		Display("Mesh Renderer").
		Icon("cat.rendering").
		Default(DefaultMeshRenderer).
		Err(); err != nil {
		return err
	}
	for _, sh := range Shapes {
		r.Template(schema.TemplateDesc{
			ID:    ID + "." + strings.ToLower(sh.Name),
			Name:  sh.Name,
			Space: schema.Space3D,
			Setup: ShapeSetup(sh.Mesh),
		})
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
	registerDev(r)
	return nil
}

func lookAtOrigin(s *scene.Scene, h scene.NodeHandle) {
	pos := mathx.Vec3{Y: 2, Z: 6}
	s.SetLocal3D(h, scene.Transform3D{Position: pos, Rotation: mathx.QuatLookDir(pos.Neg()), Scale: mathx.Vec3{X: 1, Y: 1, Z: 1}})
}

func ShapeSetup(mesh ids.AssetID) func(*scene.Scene, scene.NodeHandle) {
	return func(s *scene.Scene, h scene.NodeHandle) {
		mr := scene.Add[MeshRenderer](s, h)
		*mr = DefaultMeshRenderer()
		mr.Mesh.SetAssetRefID(mesh)
	}
}
