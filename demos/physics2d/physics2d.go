package physics2d

import (
	"strconv"
	"time"

	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/engine"
	"aego/modules/physics/world"
	"aego/platform/input"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/widgets"
)

func init() {
	demos.Register("physics2d", func() demos.Demo { return New(false) })
	demos.Register("physics2d-headless", func() demos.Demo { return New(true) })
}

var presets = [...]world.DebugFlags{
	world.DrawShapes | world.DrawJoints,
	world.DrawShapes | world.DrawJoints | world.DrawContacts,
	world.DrawShapes | world.DrawJoints | world.DrawAABBs,
	world.DrawShapes | world.DrawJoints | world.DrawIslands | world.DrawVelocities,
	world.DrawAll,
}

type Demo struct {
	app.Base
	headless bool
	mod      *renderhost.Module
	w        *world.World
	cam      view.Camera2D
	preset   int
	paused   bool
	stepOnce bool
	mouse    mathx.Vec2
	stepTime time.Duration
	steps    int
	failure  string
}

func New(headless bool) *Demo {
	return &Demo{headless: headless, cam: view.Camera2D{Zoom: 20, Position: mathx.Vec2{X: -5, Y: -12}}}
}

func (d *Demo) Name() string {
	if d.headless {
		return "physics2d-headless"
	}
	return "physics2d"
}

func (d *Demo) Configure(c *engine.Config) {
	if !d.headless {
		return
	}
	c.Headless = true
	if c.HeadlessFrameTime <= 0 {
		c.HeadlessFrameTime = 1.0 / 60
	}
}

func (d *Demo) Module() *renderhost.Module {
	if d.mod == nil {
		d.mod = renderhost.New(render.Options{MaxDebugVerts: 1 << 18})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	return d.reset()
}

func (d *Demo) reset() error {
	w, err := buildScene()
	if err != nil {
		return err
	}
	d.w, d.steps, d.failure = w, 0, ""
	return nil
}

func (d *Demo) FixedUpdate(dt float64) {
	if d.paused && !d.stepOnce {
		return
	}
	start := time.Now()
	d.w.Step(float32(dt))
	d.stepTime = time.Since(start)
	d.steps++
	d.stepOnce = false
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	switch {
	case in.KeyPressed(input.KeyEscape):
		d.Engine.Quit()
	case in.KeyPressed(input.KeyD):
		d.preset = (d.preset + 1) % len(presets)
	case in.KeyPressed(input.KeyR):
		if err := d.reset(); err != nil {
			d.failure = err.Error()
		}
	case in.KeyPressed(input.KeyP):
		d.paused = !d.paused
	case in.KeyPressed(input.KeySpace) && d.paused:
		d.stepOnce = true
	}
	if w := in.Wheel().Y; w != 0 && !d.UI.WantsMouse() {
		d.cam.Zoom = mathx.Clamp(d.cam.Zoom*(1+w*0.1), 4, 200)
	}
	speed := float32(dt) * 400 / d.cam.Zoom
	if in.KeyDown(input.KeyLeft) {
		d.cam.Position.X -= speed
	}
	if in.KeyDown(input.KeyRight) {
		d.cam.Position.X += speed
	}
	if in.KeyDown(input.KeyUp) {
		d.cam.Position.Y -= speed
	}
	if in.KeyDown(input.KeyDown) {
		d.cam.Position.Y += speed
	}
	if d.UI.WantsMouse() {
		return
	}
	if in.ButtonPressed(input.MouseLeft) {
		d.kick()
	}
	if in.ButtonPressed(input.MouseRight) {
		d.spawn()
	}
}

func (d *Demo) kick() {
	sh, ok := d.w.ShapeAt(d.mouse, world.DefaultQueryFilter())
	if !ok {
		return
	}
	b, _ := d.w.ShapeBody(sh)
	if t, _ := d.w.BodyType(b); t != world.Dynamic {
		return
	}
	mass, _, _, _ := d.w.BodyMass(b)
	_ = d.w.ApplyImpulseAtPoint(b, mathx.Vec2{Y: -8 * mass}, d.mouse)
}

func (d *Demo) spawn() {
	def := world.DefaultBodyDef()
	def.Position = d.mouse
	b, err := d.w.CreateBody(def)
	if err == nil {
		_, err = d.w.AddShape(b, world.DefaultShapeDef(world.Box{Half: mathx.Vec2{X: 0.4, Y: 0.4}}))
	}
	if err != nil {
		d.failure = err.Error()
	}
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	v.Camera = d.cam
	f := d.Renderer.View(v)
	dbg := f.Debug()
	d.w.DebugDraw(dbg, presets[d.preset])
	d.mouse = f.View().ScreenToWorld(d.Engine.Input().MousePos().Scale(d.UI.Scale()))
	origin := mathx.Vec2{X: d.cam.Position.X, Y: d.cam.Position.Y - 18}
	dir := d.mouse.Sub(origin)
	if hit, ok := d.w.Raycast(origin, dir, 200, world.DefaultQueryFilter()); ok {
		dbg.Line(origin, hit.Point, mathx.Color{R: 1, G: 0.3, B: 0.3, A: 1})
		dbg.Line(hit.Point, hit.Point.Add(hit.Normal.Scale(1.5)), mathx.Color{R: 0.3, G: 1, B: 0.4, A: 1})
		dbg.Cross(hit.Point, 0.4, mathx.Color{R: 1, G: 1, B: 1, A: 1})
	} else {
		dbg.Line(origin, origin.Add(dir.Normalized().Scale(200)), mathx.Color{R: 0.5, G: 0.5, B: 0.5, A: 0.6})
	}
	f.End()
	d.BeginUI()
	d.DrawOverlay(d.overlay)
	d.FlushUI()
}

func (d *Demo) overlay(c *core.Context) {
	s := d.w.Stats()
	widgets.LabelDim(c, "step "+strconv.FormatFloat(float64(d.stepTime.Microseconds())/1000, 'f', 2, 64)+" ms  #"+strconv.Itoa(d.steps))
	widgets.LabelDim(c, "bodies "+strconv.Itoa(s.Bodies)+"  active "+strconv.Itoa(s.Active)+"  sleeping "+strconv.Itoa(s.Sleeping))
	widgets.LabelDim(c, "pairs "+strconv.Itoa(s.TouchingPairs)+"  points "+strconv.Itoa(s.ContactPoints)+"  islands "+strconv.Itoa(s.Islands))
	widgets.LabelDim(c, "LMB kick  RMB box  D layers  P pause  Space step  R reset")
	if d.paused {
		widgets.LabelDim(c, "paused")
	}
	if d.failure != "" {
		widgets.LabelDim(c, d.failure)
	}
}

func (d *Demo) ReportMetrics(dst []demos.Metric) []demos.Metric {
	s := d.w.Stats()
	return append(dst,
		demos.Metric{Name: "physics.step_ms", Value: float64(d.stepTime.Microseconds()) / 1000},
		demos.Metric{Name: "physics.active", Value: float64(s.Active)},
		demos.Metric{Name: "physics.pairs", Value: float64(s.TouchingPairs)},
		demos.Metric{Name: "physics.islands", Value: float64(s.Islands)},
	)
}
