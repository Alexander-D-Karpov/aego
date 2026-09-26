package sprites

import (
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/demos/internal/scene"
	"aego/engine"
	"aego/gpu"
	"aego/platform/input"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

func init() {
	demos.Register("sprites", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	atlas    texture.Region
	draws    []sprite.Draw
	time     float32
	linear   bool
	bounds   bool
	nearestS gpu.SamplerHandle
	linearS  gpu.SamplerHandle
	mod      *rendermodule.Module
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "sprites"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 4096})
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
	d.nearestS = d.Renderer.Sampler(gpu.SamplerDesc{})
	d.linearS = d.Renderer.Sampler(gpu.SamplerDesc{MinFilter: gpu.FilterLinear, MagFilter: gpu.FilterLinear})
	d.draws = scene.Field(atlas, 100, 4, 42)
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	if d.Engine.Input().KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	d.time += float32(dt)
	for i := range d.draws {
		p := float32(i) * 0.37
		d.draws[i].Rotation += float32(dt) * (0.4 + p*0.02)
		s := 0.8 + 0.3*mathx.Sin(d.time+p)
		d.draws[i].Scale = mathx.Vec2{X: s, Y: s}
		d.draws[i].Tint = mathx.Color{R: 1, G: 1, B: 1, A: 0.35 + 0.65*mathx.Abs(mathx.Cos(d.time*0.5+p))}
		if d.linear {
			d.draws[i].Sampler = d.linearS
		} else {
			d.draws[i].Sampler = d.nearestS
		}
	}
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	f := d.Renderer.View(v)
	f.Sprites(d.draws)
	if d.bounds {
		dbg := f.Debug()
		for i := range d.draws {
			dbg.RectRot(d.draws[i].Position, d.draws[i].WorldSize(v.PixelsPerUnit), d.draws[i].Rotation,
				mathx.Color{R: 0.2, G: 1, B: 0.4, A: 0.6})
		}
	}
	f.End()

	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		widgets.Checkbox(c, "linear filtering", &d.linear)
		widgets.Checkbox(c, "show bounds", &d.bounds)
	})
	d.FlushUI()
}
