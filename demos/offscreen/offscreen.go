package offscreen

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
	"aego/render/view"
)

const targetSize = 256

func init() {
	demos.Register("offscreen", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	atlas  texture.Region
	target gpu.TextureHandle
	region texture.Region
	time   float32
	mod    *rendermodule.Module
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "offscreen"
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
	t, err := e.GPU().CreateTexture(gpu.TextureDesc{
		Width:  targetSize,
		Height: targetSize,
		Format: gpu.TexSRGBA8,
		Label:  "demo.target",
	})
	if err != nil {
		return err
	}
	d.target = t
	d.region = texture.Whole(t, targetSize, targetSize)
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	if d.Engine.Input().KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	d.time += float32(dt)
}

func (d *Demo) Render(float64) {
	tv := view.View{
		Target:     d.target,
		TargetW:    targetSize,
		TargetH:    targetSize,
		Viewport:   mathx.Rect{W: targetSize, H: targetSize},
		PixelScale: 1,
		Camera:     view.Camera2D{Zoom: 64},
		Clear:      mathx.Color{R: 0.1, G: 0.05, B: 0.15, A: 1},
		DoClear:    true,
		Label:      "target",
	}
	tf := d.Renderer.View(tv)
	tf.Sprite(sprite.Draw{
		Region:   scene.Cell(d.atlas, 0),
		Position: mathx.Vec2{Y: -1},
		Size:     mathx.Vec2{X: 1.5, Y: 1.5},
		Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
		Rotation: d.time,
		Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
	})
	tf.Text(d.Renderer.DebugFont(), "TOP", mathx.Vec2{X: -0.6, Y: -1.9}, 0.02,
		mathx.Color{R: 1, G: 1, B: 1, A: 1}, 1, 0)
	tf.Debug().Line(mathx.Vec2{X: -1.8, Y: -1.8}, mathx.Vec2{X: 1.8, Y: -1.8},
		mathx.Color{R: 1, G: 0.3, B: 0.2, A: 1})
	tf.Debug().Line(mathx.Vec2{X: -1.8, Y: 1.8}, mathx.Vec2{X: 1.8, Y: 1.8},
		mathx.Color{R: 0.2, G: 0.4, B: 1, A: 1})
	tf.End()

	m := d.Engine.Metrics()
	scale := m.DisplayScale
	if scale <= 0 {
		scale = 1
	}
	sv := view.ScreenView(m.PixelW, m.PixelH, scale)
	sv.Clear = mathx.Color{R: 0.04, G: 0.05, B: 0.07, A: 1}
	sv.DoClear = true
	sv.Label = "screen"
	sf := d.Renderer.View(sv)
	rect := mathx.Rect{X: 60, Y: 60, W: targetSize, H: targetSize}
	sf.Sprite(sprite.Draw{
		Region:   d.region,
		Position: mathx.Vec2{X: rect.X, Y: rect.Y},
		Size:     mathx.Vec2{X: rect.W, Y: rect.H},
		Tint:     mathx.Color{R: 1, G: 1, B: 1, A: 1},
		Blend:    sprite.BlendOpaque,
	})
	sf.Debug().Rect(rect, mathx.Color{R: 1, G: 1, B: 1, A: 0.5})
	sf.Text(d.Renderer.DebugFont(), "red line must stay on top", mathx.Vec2{X: 60, Y: 40}, 1,
		mathx.Color{R: 1, G: 1, B: 1, A: 1}, 1, 0)
	sf.End()

	d.BeginUI()
	d.DrawOverlay(nil)
	d.FlushUI()
}
