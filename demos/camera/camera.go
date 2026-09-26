package camera

import (
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/demos/internal/scene"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/widgets"
)

const tiles = 200

func init() {
	demos.Register("camera", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	atlas texture.Region
	map_  []sprite.Draw
	cam   view.Camera2D
	mode  view.ScaleMode
	mod   *rendermodule.Module
}

func New() *Demo {
	return &Demo{cam: view.Camera2D{Zoom: 32}}
}

func (d *Demo) Name() string {
	return "camera"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 65536})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	atlas, err := scene.BuildAtlas(d.Renderer)
	if err != nil {
		return err
	}
	d.atlas = atlas
	d.map_ = make([]sprite.Draw, 0, tiles*tiles)
	for y := 0; y < tiles; y++ {
		for x := 0; x < tiles; x++ {
			d.map_ = append(d.map_, sprite.Draw{
				Region:   scene.Cell(atlas, (x+y)%4),
				Position: mathx.Vec2{X: float32(x), Y: float32(y)},
				Size:     mathx.Vec2{X: 1, Y: 1},
				Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
				Layer:    0,
			})
		}
	}
	d.cam.Position = mathx.Vec2{X: tiles / 2, Y: tiles / 2}
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	speed := float32(dt) * 10 * (32 / max(d.cam.Zoom, 1))
	if in.KeyDown(input.KeyA) {
		d.cam.Position.X -= speed
	}
	if in.KeyDown(input.KeyD) {
		d.cam.Position.X += speed
	}
	if in.KeyDown(input.KeyW) {
		d.cam.Position.Y -= speed
	}
	if in.KeyDown(input.KeyS) {
		d.cam.Position.Y += speed
	}
	if in.KeyDown(input.KeyQ) {
		d.cam.Rotation -= float32(dt)
	}
	if in.KeyDown(input.KeyE) {
		d.cam.Rotation += float32(dt)
	}
	if w := in.Wheel().Y; w != 0 && !d.UI.WantsMouse() {
		d.cam.Zoom = mathx.Clamp(d.cam.Zoom*(1+w*0.1), 4, 256)
	}
	if in.KeyPressed(input.KeyTab) {
		d.mode = (d.mode + 1) % 3
	}
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	v.Camera = d.cam
	v.Scale = d.mode
	v.Virtual = mathx.Vec2{X: 640, Y: 360}
	f := d.Renderer.View(v)
	f.Sprites(d.map_)

	dbg := f.Debug()
	b := f.View().VisibleBounds()
	inner := mathx.AABB{
		Min: b.Min.Add(b.Size().Scale(0.05)),
		Max: b.Max.Sub(b.Size().Scale(0.05)),
	}
	dbg.AABB(inner, mathx.Color{R: 1, G: 0.8, B: 0.2, A: 0.8})

	mouse := d.Engine.Input().MousePos().Scale(d.UI.Scale())
	world := f.View().ScreenToWorld(mouse)
	dbg.Cross(world, 1, mathx.Color{R: 0.3, G: 1, B: 0.5, A: 1})
	dbg.Circle(world, 0.5, 24, mathx.Color{R: 0.3, G: 1, B: 0.5, A: 0.5})
	f.End()

	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		widgets.Slider(c, "zoom", &d.cam.Zoom, 4, 256)
		widgets.Slider(c, "rotation", &d.cam.Rotation, -mathx.Pi, mathx.Pi)
		switch d.mode {
		case view.ScaleFree:
			widgets.LabelDim(c, "scale: free (tab)")
		case view.ScalePixelPerfect:
			widgets.LabelDim(c, "scale: pixel perfect (tab)")
		default:
			widgets.LabelDim(c, "scale: fit virtual (tab)")
		}
	})
	d.FlushUI()
}
