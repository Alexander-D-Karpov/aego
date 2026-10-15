package scene3d

import (
	"aego/asset/loader"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/demos/internal/pipeline"
	"aego/engine"
	"aego/modules/render2d"
	"aego/modules/render3d"
	"aego/platform/input"
	"aego/render"
	"aego/render/texture"
	"aego/scene"
	"aego/scene/frame"
)

var WhiteSprite = ids.AssetID{Hi: 0xae91000000000000, Lo: 1}

type resolver struct {
	*loader.Loader
	white texture.Region
}

func (r *resolver) Texture(id ids.AssetID) (texture.Region, bool) {
	if id == WhiteSprite {
		return r.white, true
	}
	return r.Loader.Texture(id)
}

type World struct {
	Scene  *scene.Scene
	Runner *frame.Runner
	pipe   *pipeline.Pipeline
	loader *loader.Loader
}

func New(e *engine.Engine, r *render.Renderer, nodes int) (*World, error) {
	types, man, err := pipeline.Types()
	if err != nil {
		return nil, err
	}
	l := loader.New(e.GPU(), r.Textures(), e.Log())
	res := &resolver{Loader: l, white: r.White()}
	s := scene.New(types, res, scene.Options{InitialNodes: nodes, DefaultSpace: scene.Space3D})
	pipe := pipeline.New(man)
	pipe.Runner.Bind(s, r, res)
	return &World{Scene: s, Runner: pipe.Runner, pipe: pipe, loader: l}, nil
}

func (w *World) Camera(pos, target mathx.Vec3) scene.NodeHandle {
	s := w.Scene
	h := s.Create3D(s.Root(), "Camera")
	*scene.Add[render3d.Camera3D](s, h) = render3d.DefaultCamera3D()
	Aim(s, h, pos, target)
	return h
}

func Aim(s *scene.Scene, h scene.NodeHandle, pos, target mathx.Vec3) {
	s.SetLocal3D(h, scene.Transform3D{
		Position: pos,
		Rotation: mathx.QuatLookDir(target.Sub(pos)),
		Scale:    mathx.Vec3{X: 1, Y: 1, Z: 1},
	})
}

func (w *World) Mesh(parent scene.NodeHandle, name string, mesh ids.AssetID, pos mathx.Vec3, color mathx.Color) scene.NodeHandle {
	s := w.Scene
	h := s.Create3D(parent, name)
	mr := scene.Add[render3d.MeshRenderer](s, h)
	*mr = render3d.DefaultMeshRenderer()
	mr.Mesh.SetAssetRefID(mesh)
	mr.Color = color
	s.SetPosition3D(h, pos)
	return h
}

func (w *World) Sprite(parent scene.NodeHandle, name string, pos, size mathx.Vec2, tint mathx.Color) scene.NodeHandle {
	s := w.Scene
	h, err := s.CreateIn(parent, name, ids.NodeID{}, scene.Space2D)
	if err != nil {
		panic(err)
	}
	sr := scene.Add[render2d.SpriteRenderer](s, h)
	*sr = render2d.DefaultSpriteRenderer()
	sr.Sprite.SetAssetRefID(WhiteSprite)
	sr.Size = size
	sr.Tint = tint
	sr.Origin = mathx.Vec2{X: 0.5, Y: 0.5}
	s.SetPosition(h, pos)
	return h
}

func (w *World) Render(e *engine.Engine, r *render.Renderer) {
	w.pipe.Render(e, r)
}

func (w *World) Close() {
	if w.loader != nil {
		w.loader.Close()
	}
}

type Orbit struct {
	Target   mathx.Vec3
	Distance float32
	Yaw      float32
	Pitch    float32
}

func (o *Orbit) Update(in *input.State, dt float32) {
	const turn, zoom = 1.5, 6
	if in.KeyDown(input.KeyA) {
		o.Yaw -= turn * dt
	}
	if in.KeyDown(input.KeyD) {
		o.Yaw += turn * dt
	}
	if in.KeyDown(input.KeyW) {
		o.Pitch = min(o.Pitch+turn*dt, 1.5)
	}
	if in.KeyDown(input.KeyS) {
		o.Pitch = max(o.Pitch-turn*dt, -1.5)
	}
	if in.KeyDown(input.KeyQ) {
		o.Distance = max(o.Distance-zoom*dt, 1)
	}
	if in.KeyDown(input.KeyE) {
		o.Distance += zoom * dt
	}
}

func (o *Orbit) Eye() mathx.Vec3 {
	cp := mathx.Cos(o.Pitch)
	dir := mathx.Vec3{X: mathx.Sin(o.Yaw) * cp, Y: mathx.Sin(o.Pitch), Z: mathx.Cos(o.Yaw) * cp}
	return o.Target.Add(dir.Scale(o.Distance))
}

func (o *Orbit) Apply(s *scene.Scene, cam scene.NodeHandle) {
	Aim(s, cam, o.Eye(), o.Target)
}
