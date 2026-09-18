package resize

import (
	"aego/core/log"
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/overlay"
	"aego/demos/internal/quad"
	"aego/engine"
	"aego/gpu"
	"aego/platform"
	"aego/platform/input"
)

const (
	worldHalfW = 8
	worldHalfH = 4.5
	markerSize = 0.4
)

func init() {
	demos.Register("resize", func() demos.Demo { return New() })
}

type Demo struct {
	demos.Base
	e          *engine.Engine
	log        *log.ChannelLogger
	quad       *quad.Renderer
	overlay    *overlay.Overlay
	last       platform.WindowMetrics
	fullscreen bool
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "resize"
}

func (d *Demo) Init(e *engine.Engine) error {
	d.e = e
	d.log = e.Log().With(log.Game)
	q, err := quad.New(e.GPU(), e.VFS(), 1)
	if err != nil {
		return err
	}
	d.quad = q
	d.last = e.Metrics()
	d.fullscreen = e.Config().Fullscreen
	d.overlay = overlay.New(e)
	return nil
}

func (d *Demo) Event(ev platform.Event) {
	if ev.Kind != platform.EvWindowResized || ev.Window == d.last {
		return
	}
	d.last = ev.Window
	d.log.Info("window metrics changed",
		log.F("logical_w", ev.Window.LogicalW), log.F("logical_h", ev.Window.LogicalH),
		log.F("pixel_w", ev.Window.PixelW), log.F("pixel_h", ev.Window.PixelH),
		log.F("density", ev.Window.PixelDensity), log.F("display_scale", ev.Window.DisplayScale))
}

func (d *Demo) Update(dt float64) {
	d.overlay.Update(dt)
	in := d.e.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.e.Quit()
	}
	if in.KeyPressed(input.KeyF) {
		d.fullscreen = !d.fullscreen
		d.e.Window().SetFullscreen(d.fullscreen)
	}
}

func fitViewport(pw, ph int, aspect float32) mathx.Rect {
	w, h := float32(pw), float32(ph)
	if w/h > aspect {
		vw := h * aspect
		return mathx.Rect{X: (w - vw) / 2, W: vw, H: h}
	}
	vh := w / aspect
	return mathx.Rect{Y: (h - vh) / 2, W: w, H: vh}
}

func (d *Demo) Render(float64) {
	fr := d.e.Frame()
	if fr == nil {
		return
	}
	m := d.e.Metrics()
	vp := fitViewport(m.PixelW, m.PixelH, worldHalfW/worldHalfH)
	proj := mathx.Mat4Ortho(-worldHalfW, worldHalfW, worldHalfH, -worldHalfH, -1, 1)

	p := fr.BeginPass(gpu.PassDesc{
		Color:      gpu.LoadClear,
		ClearColor: mathx.Color{R: 0.01, G: 0.01, B: 0.01, A: 1},
		Label:      "resize",
	})
	p.SetViewport(vp)
	p.SetScissor(vp, true)
	d.drawQuad(p, proj, mathx.Vec2{}, mathx.Vec2{X: worldHalfW, Y: worldHalfH},
		mathx.Color{R: 0.10, G: 0.12, B: 0.16, A: 1})
	for _, corner := range [4]mathx.Vec2{
		{X: -worldHalfW + markerSize, Y: -worldHalfH + markerSize},
		{X: worldHalfW - markerSize, Y: -worldHalfH + markerSize},
		{X: worldHalfW - markerSize, Y: worldHalfH - markerSize},
		{X: -worldHalfW + markerSize, Y: worldHalfH - markerSize},
	} {
		d.drawQuad(p, proj, corner, mathx.Vec2{X: markerSize, Y: markerSize},
			mathx.Color{R: 0.9, G: 0.5, B: 0.2, A: 1})
	}
	d.drawQuad(p, proj, mathx.Vec2{}, mathx.Vec2{X: 1.5, Y: 1.5},
		mathx.Color{R: 1, G: 1, B: 1, A: 1})
	p.SetScissor(vp, false)
	p.End()
}

func (d *Demo) drawQuad(p gpu.Pass, proj mathx.Mat4, pos, scale mathx.Vec2, tint mathx.Color) {
	model := mathx.Mat4FromMat3(mathx.Mat3TRS(pos, 0, scale))
	d.quad.Draw(p, proj.Mul(model), tint)
}

func (d *Demo) Shutdown() {
	if d.quad != nil {
		d.quad.Destroy()
	}
}
