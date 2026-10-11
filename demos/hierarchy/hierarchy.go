package hierarchy

import (
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/demos/internal/pipeline"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/render/sprite"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

func init() {
	demos.Register("hierarchy", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod    *rendermodule.Module
	scene  *scene.Scene
	ship   scene.NodeHandle
	turret scene.NodeHandle
	gun    scene.NodeHandle
	time   float32
	axes   bool
}

func New() *Demo {
	return &Demo{axes: true}
}

func (d *Demo) Name() string {
	return "hierarchy"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 1024})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	types, _, err := pipeline.Types()
	if err != nil {
		return err
	}
	s := scene.New(types, nil, scene.Options{})
	d.scene = s

	d.ship = s.Create(s.Root(), "Spaceship")
	body := s.Create(d.ship, "Body")
	d.turret = s.Create(body, "Turret")
	d.gun = s.Create(d.turret, "Gun")
	engineNode := s.Create(body, "Engine")

	s.SetLocal(body, scene.Transform2D{Scale: mathx.Vec2{X: 1, Y: 1}})
	s.SetLocal(d.turret, scene.Transform2D{Position: mathx.Vec2{Y: -1.2}, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.SetLocal(d.gun, scene.Transform2D{Position: mathx.Vec2{Y: -1}, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.SetLocal(engineNode, scene.Transform2D{Position: mathx.Vec2{Y: 1.4}, Scale: mathx.Vec2{X: 1, Y: 1}})
	s.Flush()
	s.UpdateTransforms()
	return nil
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if in.KeyPressed(input.KeyT) {
		d.scene.SetEnabled(d.turret, !d.scene.Enabled(d.turret))
	}
	if in.KeyPressed(input.KeyB) {
		d.axes = !d.axes
	}
	d.time += float32(dt)
	s := d.scene
	s.SetLocal(d.ship, scene.Transform2D{
		Position: mathx.Vec2{X: mathx.Sin(d.time*0.5) * 3, Y: mathx.Cos(d.time*0.3) * 1.5},
		Rotation: d.time * 0.4,
		Scale:    mathx.Vec2{X: 1, Y: 1},
	})
	s.SetRotation(d.turret, -d.time*1.3)
	s.Flush()
	s.UpdateTransforms()
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	v.Camera.Zoom = 48
	f := d.Renderer.View(v)
	white := d.Renderer.White()
	dbg := f.Debug()

	sizes := map[string]mathx.Vec2{
		"Spaceship": {X: 0.4, Y: 0.4},
		"Body":      {X: 1.6, Y: 2.4},
		"Turret":    {X: 1, Y: 0.8},
		"Gun":       {X: 0.3, Y: 1.4},
		"Engine":    {X: 1, Y: 0.7},
	}
	tints := map[string]mathx.Color{
		"Spaceship": {R: 1, G: 1, B: 1, A: 1},
		"Body":      {R: 0.32, G: 0.65, B: 0.79, A: 1},
		"Turret":    {R: 0.88, G: 0.48, B: 0.22, A: 1},
		"Gun":       {R: 0.95, G: 0.85, B: 0.3, A: 1},
		"Engine":    {R: 0.54, G: 0.79, B: 0.32, A: 1},
	}

	s := d.scene
	for _, h := range s.PreOrder() {
		if h == s.Root() || !s.EffectiveEnabled(h) {
			continue
		}
		name := s.Name(h)
		world := s.World(h)
		pos := mathx.Vec2{X: world[6], Y: world[7]}
		rot := mathx.Atan2(world[1], world[0])
		f.Sprite(sprite.Draw{
			Region:   white,
			Position: pos,
			Size:     sizes[name],
			Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
			Rotation: rot,
			Tint:     tints[name],
			Layer:    1,
		})
		f.Text(d.Renderer.DebugFont(), name, pos.Add(mathx.Vec2{X: 0.3, Y: -0.3}), 0.02,
			mathx.Color{R: 1, G: 1, B: 1, A: 0.8}, 2, 0)
		if d.axes {
			x := mathx.Vec2{X: world[0], Y: world[1]}.Normalized().Scale(0.8)
			y := mathx.Vec2{X: world[3], Y: world[4]}.Normalized().Scale(0.8)
			dbg.Line(pos, pos.Add(x), mathx.Color{R: 1, G: 0.2, B: 0.2, A: 1})
			dbg.Line(pos, pos.Add(y), mathx.Color{R: 0.2, G: 1, B: 0.3, A: 1})
		}
	}
	f.End()

	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		widgets.LabelDim(c, "T toggles the turret, B toggles axes")
		enabled := d.scene.Enabled(d.turret)
		if widgets.Checkbox(c, "turret enabled", &enabled) {
			d.scene.SetEnabled(d.turret, enabled)
		}
		widgets.Checkbox(c, "world axes", &d.axes)
		widgets.Labelf(c, "gun effective: %v", d.scene.EffectiveEnabled(d.gun))
	})
	d.FlushUI()
}
