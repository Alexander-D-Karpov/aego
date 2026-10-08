package meshdemo

import (
	"aego/asset"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/demos/internal/scene3d"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

func init() {
	demos.Register("mesh", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod    *rendermodule.Module
	world  *scene3d.World
	cam    scene.NodeHandle
	orbit  scene3d.Orbit
	spin   []scene.NodeHandle
	time   float32
	paused bool
}

func New() *Demo {
	return &Demo{orbit: scene3d.Orbit{Target: mathx.Vec3{Y: 0.5}, Distance: 9, Yaw: 0.6, Pitch: 0.45}}
}

func (d *Demo) Name() string {
	return "mesh"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 1024})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	w, err := scene3d.New(e, d.Renderer, 64)
	if err != nil {
		return err
	}
	d.world = w
	root := w.Scene.Root()
	w.Mesh(root, "Ground", asset.BuiltinPlane, mathx.Vec3{}, mathx.Color{R: 0.18, G: 0.2, B: 0.24, A: 1})
	shapes := []struct {
		name  string
		mesh  ids.AssetID
		color mathx.Color
	}{
		{"Cube", asset.BuiltinCube, mathx.Color{R: 0.88, G: 0.48, B: 0.22, A: 1}},
		{"Sphere", asset.BuiltinSphere, mathx.Color{R: 0.32, G: 0.65, B: 0.79, A: 1}},
		{"Cylinder", asset.BuiltinCylinder, mathx.Color{R: 0.54, G: 0.79, B: 0.32, A: 1}},
		{"Crate", asset.BuiltinCube, mathx.Color{R: 0.95, G: 0.85, B: 0.3, A: 1}},
	}
	for i, sh := range shapes {
		x := float32(i)*2 - 3
		d.spin = append(d.spin, w.Mesh(root, sh.name, sh.mesh, mathx.Vec3{X: x, Y: 0.5}, sh.color))
	}
	d.cam = w.Camera(d.orbit.Eye(), d.orbit.Target)
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if in.KeyPressed(input.KeySpace) {
		d.paused = !d.paused
	}
	d.orbit.Update(in, float32(dt))
	s := d.world.Scene
	d.orbit.Apply(s, d.cam)
	if d.paused {
		return
	}
	d.time += float32(dt)
	for i, h := range d.spin {
		f := float32(i)
		s.SetRotation3D(h, mathx.QuatEuler(d.time*0.7+f, d.time+f*0.5, 0))
	}
}

func (d *Demo) Render(float64) {
	d.world.Render(d.Engine, d.Renderer)
	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		st := d.world.Runner.Stats()
		rs := d.Module().Stats()
		widgets.LabelDim(c, "A/D yaw, W/S pitch, Q/E zoom, Space pause")
		widgets.Labelf(c, "views %d meshes %d", st.Views, st.Meshes)
		widgets.Labelf(c, "draw calls %d mesh draws %d culled %d", rs.DrawCalls, rs.MeshDraws, rs.MeshCulled)
	})
	d.FlushUI()
}

func (d *Demo) Shutdown() {
	if d.world != nil {
		d.world.Close()
	}
}
