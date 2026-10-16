package stress

import (
	"aego/core/mathx"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/demos/internal/scene"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/sprite"
	"aego/render/texture"
	"aego/ui/core"
	"aego/ui/widgets"
)

const (
	spriteCount = 50000
	extent      = 60
)

func init() {
	demos.Register("stress", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	atlas  texture.Region
	draws  []sprite.Draw
	time   float32
	spin   bool
	mod    *renderhost.Module
	custom customPass
}

func New() *Demo {
	return &Demo{spin: true}
}

func (d *Demo) Name() string {
	return "stress"
}

func (d *Demo) Module() *renderhost.Module {
	if d.mod == nil {
		d.mod = renderhost.New(render.Options{MaxSprites: spriteCount + 4096})
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
	d.draws = scene.Field(atlas, spriteCount, extent, 1337)
	for i := range d.draws {
		d.draws[i].Region = scene.Cell(atlas, i%4)
		d.draws[i].Layer = 0
		d.draws[i].Order = 0
	}
	return d.Renderer.AddPass(render.PassDesc{
		ID:    "demo.custom",
		After: []string{"debug"},
		Pass:  &d.custom,
	})
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if in.KeyPressed(input.KeySpace) {
		d.spin = !d.spin
	}
	d.time += float32(dt)
	if !d.spin {
		return
	}
	for i := range d.draws {
		d.draws[i].Rotation += float32(dt) * 0.5
	}
}

func (d *Demo) Render(float64) {
	v := d.WorldView()
	v.Camera.Zoom = 24
	v.Camera.Position = mathx.Vec2{
		X: mathx.Sin(d.time*0.2) * extent * 0.6,
		Y: mathx.Cos(d.time*0.15) * extent * 0.6,
	}
	f := d.Renderer.View(v)
	f.Sprites(d.draws)
	f.Debug().AABB(f.View().VisibleBounds(), mathx.Color{R: 1, G: 0.9, B: 0.3, A: 0.35})
	f.End()

	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		widgets.Checkbox(c, "rotate (space)", &d.spin)
		widgets.LabelDim(c, "custom pass draws the corner mark")
	})
	d.FlushUI()
}

type customPass struct {
	calls uint32
}

func (p *customPass) Execute(ctx *render.PassContext) {
	p.calls++
}
