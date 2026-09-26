package layers

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
)

func init() {
	demos.Register("layers", func() demos.Demo { return New() })
}

var names = [5]string{"background", "player", "smoke", "foreground", "ui-in-world"}

type Demo struct {
	app.Base
	atlas   texture.Region
	time    float32
	reverse bool
	mod     *rendermodule.Module
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "layers"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 1024})
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
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if in.KeyPressed(input.KeyR) {
		d.reverse = !d.reverse
	}
	d.time += float32(dt)
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	v.Camera.Zoom = 96
	f := d.Renderer.View(v)

	for i := 0; i < 5; i++ {
		layer := int16(i)
		if d.reverse {
			layer = int16(4 - i)
		}
		x := -2.4 + float32(i)*1.1 + 0.4*mathx.Sin(d.time+float32(i))
		f.Sprite(sprite.Draw{
			Region:   scene.Cell(d.atlas, i%4),
			Position: mathx.Vec2{X: x, Y: 0},
			Size:     mathx.Vec2{X: 2, Y: 2},
			Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
			Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 0.65},
			Layer:    layer,
		})
		f.Text(d.Renderer.DebugFont(), names[i],
			mathx.Vec2{X: x - 0.5, Y: 1.2}, 0.012,
			mathx.Color{R: 1, G: 1, B: 1, A: 1}, layer, 100)
	}
	f.End()

	d.BeginUI()
	d.DrawOverlay(nil)
	d.FlushUI()
}
