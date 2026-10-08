package stressmesh

import (
	"aego/asset"
	"aego/core/mathx"
	"aego/core/rng"
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

const (
	gridX   = 25
	gridY   = 20
	gridZ   = 20
	spacing = 1.6
)

func init() {
	demos.Register("stress-mesh", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod       *rendermodule.Module
	world     *scene3d.World
	cam       scene.NodeHandle
	orbit     scene3d.Orbit
	cubes     []scene.NodeHandle
	time      float32
	move      bool
	transform float64
	extract   float64
}

func New() *Demo {
	return &Demo{move: true, orbit: scene3d.Orbit{Distance: 55, Pitch: 0.35}}
}

func (d *Demo) Name() string {
	return "stress-mesh"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 1024})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	n := gridX * gridY * gridZ
	w, err := scene3d.New(e, d.Renderer, n+8)
	if err != nil {
		return err
	}
	d.world = w
	r := rng.New(7)
	root := w.Scene.Root()
	off := mathx.Vec3{X: gridX - 1, Y: gridY - 1, Z: gridZ - 1}.Scale(spacing / 2)
	d.cubes = make([]scene.NodeHandle, 0, n)
	for x := 0; x < gridX; x++ {
		for y := 0; y < gridY; y++ {
			for z := 0; z < gridZ; z++ {
				p := mathx.Vec3{X: float32(x), Y: float32(y), Z: float32(z)}.Scale(spacing).Sub(off)
				c := mathx.Color{R: r.Range(0.3, 1), G: r.Range(0.3, 1), B: r.Range(0.3, 1), A: 1}
				d.cubes = append(d.cubes, w.Mesh(root, "cube", asset.BuiltinCube, p, c))
			}
		}
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
		d.move = !d.move
	}
	s := d.world.Scene
	d.orbit.Update(in, float32(dt))
	d.orbit.Yaw += float32(dt) * 0.1
	d.orbit.Apply(s, d.cam)
	if !d.move {
		return
	}
	d.time += float32(dt)
	start := d.Engine.Platform().Now()
	for i := 0; i < len(d.cubes); i += 10 {
		s.SetRotation3D(d.cubes[i], mathx.QuatEuler(d.time, d.time*0.7+float32(i), 0))
	}
	s.UpdateTransforms()
	d.transform = d.Engine.Platform().Now() - start
}

func (d *Demo) Render(float64) {
	start := d.Engine.Platform().Now()
	d.world.Render(d.Engine, d.Renderer)
	d.extract = d.Engine.Platform().Now() - start
	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		st := d.world.Runner.Stats()
		rs := d.Module().Stats()
		widgets.Labelf(c, "cubes %d submitted %d drawn %d culled %d", len(d.cubes), st.Meshes, rs.MeshDraws, rs.MeshCulled)
		widgets.Labelf(c, "draw calls %d", rs.DrawCalls)
		widgets.Labelf(c, "transform %.2f ms extract %.2f ms", d.transform*1000, d.extract*1000)
		widgets.Checkbox(c, "animate (space)", &d.move)
	})
	d.FlushUI()
}

func (d *Demo) Shutdown() {
	if d.world != nil {
		d.world.Close()
	}
}
