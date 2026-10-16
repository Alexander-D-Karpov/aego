package stressscene

import (
	"aego/core/mathx"
	"aego/core/rng"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/demos/internal/pipeline"
	"aego/engine"
	"aego/modules/render2d"
	"aego/platform/input"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/sprite"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	parents  = 5000
	children = 9
	extent   = 80
)

func init() {
	demos.Register("stress-scene", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod       *renderhost.Module
	scene     *scene.Scene
	roots     []scene.NodeHandle
	white     bool
	time      float32
	move      bool
	transform float64
	extract   float64
}

func New() *Demo {
	return &Demo{move: true}
}

func (d *Demo) Name() string {
	return "stress-scene"
}

func (d *Demo) ReportMetrics(dst []demos.Metric) []demos.Metric {
	return append(dst,
		demos.Metric{Name: "transformMs", Value: d.transform * 1000},
		demos.Metric{Name: "extractMs", Value: d.extract * 1000},
	)
}

func (d *Demo) Module() *renderhost.Module {
	if d.mod == nil {
		d.mod = renderhost.New(render.Options{MaxSprites: 65536})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	types, _, err := pipeline.Types()
	if err != nil {
		return err
	}
	s := scene.New(types, nil, scene.Options{InitialNodes: parents * (children + 1)})
	d.scene = s
	r := rng.New(99)
	white := d.Renderer.White()

	d.roots = make([]scene.NodeHandle, 0, parents)
	for i := 0; i < parents; i++ {
		p := s.Create(s.Root(), "group")
		s.SetPosition(p, mathx.Vec2{
			X: r.Range(-extent, extent),
			Y: r.Range(-extent, extent),
		})
		d.roots = append(d.roots, p)
		for j := 0; j < children; j++ {
			c := s.Create(p, "sprite")
			angle := float32(j) / float32(children) * mathx.TwoPi
			s.SetLocal(c, scene.Transform2D{
				Position: mathx.Vec2{X: mathx.Cos(angle) * 1.5, Y: mathx.Sin(angle) * 1.5},
				Scale:    mathx.Vec2{X: 1, Y: 1},
			})
			sr := scene.Add[render2d.SpriteRenderer](s, c)
			*sr = render2d.DefaultSpriteRenderer()
			sr.Size = mathx.Vec2{X: 0.8, Y: 0.8}
			sr.Tint = mathx.Color{
				R: r.Range(0.3, 1),
				G: r.Range(0.3, 1),
				B: r.Range(0.3, 1),
				A: 1,
			}
		}
	}
	s.Flush()
	s.UpdateTransforms()
	d.white = !white.IsZero()
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
	d.time += float32(dt)
	if !d.move {
		return
	}
	start := d.Engine.Platform().Now()
	step := len(d.roots) / 10
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(d.roots); i += step {
		d.scene.SetRotation(d.roots[i], d.time+float32(i))
	}
	d.scene.UpdateTransforms()
	d.transform = d.Engine.Platform().Now() - start
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	v.Camera.Zoom = 12
	v.Camera.Position = mathx.Vec2{
		X: mathx.Sin(d.time*0.15) * extent * 0.5,
		Y: mathx.Cos(d.time*0.1) * extent * 0.5,
	}
	start := d.Engine.Platform().Now()
	f := d.Renderer.View(v)
	white := d.Renderer.White()
	s := d.scene
	store := scene.Store[render2d.SpriteRenderer](s)
	store.Each(func(h scene.NodeHandle, sr *render2d.SpriteRenderer) {
		world := s.World(h)
		f.Sprite(sprite.Draw{
			Region:   white,
			Position: mathx.Vec2{X: world[6], Y: world[7]},
			Size:     sr.Size,
			Origin:   sr.Origin,
			Rotation: mathx.Atan2(world[1], world[0]),
			Tint:     sr.Tint,
			Layer:    0,
		})
	})
	f.End()
	d.extract = d.Engine.Platform().Now() - start

	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		st := s.Stats()
		widgets.Labelf(c, "nodes %d components %d", st.Alive, st.Components)
		widgets.Labelf(c, "transform %.2f ms", d.transform*1000)
		widgets.Labelf(c, "extract %.2f ms", d.extract*1000)
		widgets.Checkbox(c, "animate (space)", &d.move)
	})
	d.FlushUI()
}
