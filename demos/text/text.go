package text

import (
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/render/view"
)

func init() {
	demos.Register("text", func() demos.Demo { return New() })
}

const sample = "The quick brown fox jumps over the lazy dog\n" +
	"0123456789 !\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~\n" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ\n" +
	"abcdefghijklmnopqrstuvwxyz"

type Demo struct {
	app.Base
	scale float32
	mod   *rendermodule.Module
}

func New() *Demo {
	return &Demo{scale: 2}
}

func (d *Demo) Name() string {
	return "text"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 4096})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	return nil
}

func (d *Demo) Update(float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if w := in.Wheel().Y; w != 0 {
		d.scale = mathx.Clamp(d.scale+w*0.25, 0.5, 8)
	}
}

func (d *Demo) Render(float64) {
	m := d.Engine.Metrics()
	scale := m.DisplayScale
	if scale <= 0 {
		scale = 1
	}
	v := view.ScreenView(m.PixelW, m.PixelH, scale)
	v.Clear = mathx.Color{R: 0.05, G: 0.05, B: 0.06, A: 1}
	v.DoClear = true
	v.Label = "text"

	font := d.Renderer.DebugFont()
	f := d.Renderer.View(v)
	if font != nil {
		pos := mathx.Vec2{X: 40, Y: 60}
		f.Text(font, sample, pos, d.scale, mathx.Color{R: 1, G: 1, B: 1, A: 1}, 1, 0)
		size := font.Measure(sample).Scale(d.scale)
		f.Debug().Space(0)
		f.Debug().Rect(mathx.Rect{X: pos.X, Y: pos.Y, W: size.X, H: size.Y},
			mathx.Color{R: 1, G: 0.4, B: 0.2, A: 0.9})
	}
	f.End()

	d.BeginUI()
	d.DrawOverlay(nil)
	d.FlushUI()
}
