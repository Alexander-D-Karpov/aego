package mixed

import (
	"aego/asset"
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

const ring = 12

func init() {
	demos.Register("mixed", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod   *rendermodule.Module
	world *scene3d.World
	cam   scene.NodeHandle
	orbit scene3d.Orbit
	board scene.NodeHandle
	wheel scene.NodeHandle
	moon  scene.NodeHandle
	block scene.NodeHandle
	time  float32
}

func New() *Demo {
	return &Demo{orbit: scene3d.Orbit{Target: mathx.Vec3{Y: 1.5}, Distance: 10, Yaw: 0.4, Pitch: 0.3}}
}

func (d *Demo) Name() string {
	return "mixed"
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
	s := w.Scene
	root := s.Root()
	w.Mesh(root, "Ground", asset.BuiltinPlane, mathx.Vec3{}, mathx.Color{R: 0.18, G: 0.2, B: 0.24, A: 1})
	d.board = s.Create3D(root, "Board")
	s.SetPosition3D(d.board, mathx.Vec3{Y: 1.5})
	d.wheel = w.Sprite(d.board, "Wheel", mathx.Vec2{}, mathx.Vec2{X: 0.4, Y: 0.4}, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	for i := 0; i < ring; i++ {
		a := float32(i) / ring * mathx.TwoPi
		t := float32(i) / ring
		tint := mathx.Color{R: 0.4 + 0.6*t, G: 0.5, B: 1 - 0.6*t, A: 1}
		w.Sprite(d.wheel, "Spoke", mathx.Vec2{X: mathx.Cos(a) * 1.4, Y: mathx.Sin(a) * 1.4}, mathx.Vec2{X: 0.35, Y: 0.35}, tint)
	}
	d.moon = w.Sprite(d.wheel, "Moon", mathx.Vec2{X: 0.7}, mathx.Vec2{X: 0.2, Y: 0.2}, mathx.Color{R: 0.95, G: 0.85, B: 0.3, A: 1})
	d.block = w.Mesh(root, "Block", asset.BuiltinCube, mathx.Vec3{Y: 1.5}, mathx.Color{R: 0.88, G: 0.48, B: 0.22, A: 1})
	d.cam = w.Camera(d.orbit.Eye(), d.orbit.Target)
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	d.time += float32(dt)
	s := d.world.Scene
	d.orbit.Update(in, float32(dt))
	d.orbit.Apply(s, d.cam)
	s.SetRotation3D(d.board, mathx.QuatAxisAngle(mathx.Vec3{Y: 1}, d.time*0.5))
	s.SetRotation(d.wheel, d.time)
	s.SetRotation(d.moon, -d.time*3)
	s.SetPosition3D(d.block, mathx.Vec3{Y: 1.5, Z: mathx.Sin(d.time) * 1.2})
}

func (d *Demo) Render(float64) {
	d.world.Render(d.Engine, d.Renderer)
	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		st := d.world.Runner.Stats()
		widgets.LabelDim(c, "2D sprites on a rotating 3D board; the cube passes through them")
		widgets.Labelf(c, "meshes %d sprites3d %d", st.Meshes, st.Sprites3D)
	})
	d.FlushUI()
}

func (d *Demo) Shutdown() {
	if d.world != nil {
		d.world.Close()
	}
}
