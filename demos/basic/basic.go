package basic

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
	viewHalfHeight = 3
	moveSpeed      = 4
	spinSpeed      = 0.8
)

func init() {
	demos.Register("basic", func() demos.Demo { return New() })
}

type Demo struct {
	demos.Base
	e          *engine.Engine
	log        *log.ChannelLogger
	quad       *quad.Renderer
	overlay    *overlay.Overlay
	pos        mathx.Vec2
	prevPos    mathx.Vec2
	angle      float32
	prevAngle  float32
	vsync      platform.VSyncMode
	fullscreen bool
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "basic"
}

func (d *Demo) Init(e *engine.Engine) error {
	d.e = e
	d.log = e.Log().With(log.Game)
	q, err := quad.New(e.GPU(), e.VFS(), 2)
	if err != nil {
		return err
	}
	d.quad = q
	d.vsync = e.VSync()
	d.fullscreen = e.Config().Fullscreen
	d.overlay = overlay.New(e)
	return nil
}

func (d *Demo) FixedUpdate(dt float64) {
	d.prevPos, d.prevAngle = d.pos, d.angle
	in := d.e.Input()
	var dir mathx.Vec2
	if in.KeyDown(input.KeyLeft) || in.KeyDown(input.KeyA) {
		dir.X--
	}
	if in.KeyDown(input.KeyRight) || in.KeyDown(input.KeyD) {
		dir.X++
	}
	if in.KeyDown(input.KeyUp) || in.KeyDown(input.KeyW) {
		dir.Y--
	}
	if in.KeyDown(input.KeyDown) || in.KeyDown(input.KeyS) {
		dir.Y++
	}
	if dir != (mathx.Vec2{}) {
		d.pos = d.pos.Add(dir.Normalized().Scale(moveSpeed * float32(dt)))
	}
	d.angle += spinSpeed * float32(dt)
	if d.angle > mathx.TwoPi {
		d.angle -= mathx.TwoPi
		d.prevAngle -= mathx.TwoPi
	}
}

func (d *Demo) Update(dt float64) {
	d.overlay.Update(dt)
	in := d.e.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.e.Quit()
	}
	if in.KeyPressed(input.KeyV) {
		next := platform.VSyncMode((int(d.vsync) + 1) % 3)
		applied := d.e.SetVSync(next)
		d.log.Info("vsync changed", log.F("requested", next.String()), log.F("applied", applied.String()))
		d.vsync = next
	}
	if in.KeyPressed(input.KeyF) {
		d.fullscreen = !d.fullscreen
		d.e.Window().SetFullscreen(d.fullscreen)
	}
}

func (d *Demo) Render(alpha float64) {
	fr := d.e.Frame()
	if fr == nil {
		return
	}
	m := d.e.Metrics()
	a := float32(alpha)
	pos := d.prevPos.Lerp(d.pos, a)
	angle := mathx.Lerp(d.prevAngle, d.angle, a)
	hh := float32(viewHalfHeight)
	hw := hh * m.Aspect()
	proj := mathx.Mat4Ortho(-hw, hw, hh, -hh, -1, 1)
	model := mathx.Mat4FromMat3(mathx.Mat3TRS(pos, angle, mathx.Vec2{X: 1, Y: 1}))

	p := fr.BeginPass(gpu.PassDesc{
		Color:      gpu.LoadClear,
		ClearColor: mathx.Color{R: 0.04, G: 0.05, B: 0.07, A: 1},
		Label:      "basic",
	})
	p.SetViewport(mathx.Rect{W: float32(m.PixelW), H: float32(m.PixelH)})
	d.quad.Draw(p, proj.Mul(model), mathx.Color{R: 1, G: 1, B: 1, A: 1})
	p.End()
}

func (d *Demo) Shutdown() {
	if d.quad != nil {
		d.quad.Destroy()
	}
}
