package sceneload

import (
	"aego/core/log"
	"aego/demos"
	"aego/demos/internal/app"
	"aego/engine"
	"aego/platform/input"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/scene/systems"
	"aego/schema"
	"aego/ui/core"
	"aego/ui/scenedebug"
	"aego/ui/widgets"
)

const scenePath = "res://scenes/test.aescene"

func init() {
	demos.Register("scene-load", func() demos.Demo { return New() })
}

type Demo struct {
	app.Base
	mod    *rendermodule.Module
	scene  *scene.Scene
	runner *systems.Runner
	panel  *scenedebug.Panel
	err    string
	show   bool
	time   float32
}

func New() *Demo {
	return &Demo{}
}

func (d *Demo) Name() string {
	return "scene-load"
}

func (d *Demo) Module() *rendermodule.Module {
	if d.mod == nil {
		d.mod = rendermodule.New(render.Options{MaxSprites: 16384})
	}
	return d.mod
}

func (d *Demo) Init(e *engine.Engine) error {
	d.Attach(e, d.Module())
	return d.reload()
}

func (d *Demo) reload() error {
	src, err := d.Engine.VFS().ReadFile(scenePath)
	if err != nil {
		d.err = err.Error()
		return nil
	}
	types := newTypes()
	res := &stubResolver{region: d.Renderer.White()}
	s, report, err := sceneio.Load(src, scenePath, types, res, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		d.err = err.Error()
		return nil
	}
	d.err = ""
	for _, w := range report.Warnings {
		d.Engine.Log().With(log.Game).Warn("scene warning", log.F("msg", w.Msg))
	}
	d.scene = s
	d.runner = systems.New()
	d.runner.Bind(s, d.Renderer, res)
	d.panel = scenedebug.New(s, res)
	return nil
}

func newTypes() *scene.Types {
	t := scene.NewTypes(schema.NewRegistry())
	components.Register(t)
	sceneio.RegisterUnknown(t)
	t.Schema().Freeze()
	return t
}

func (d *Demo) Update(dt float64) {
	d.ToggleOverlay()
	in := d.Engine.Input()
	if in.KeyPressed(input.KeyEscape) {
		d.Engine.Quit()
	}
	if in.KeyPressed(input.KeyF5) {
		d.reload()
	}
	if in.KeyPressed(input.KeyF2) {
		d.show = !d.show
	}
	if d.scene == nil {
		return
	}
	d.time += float32(dt)
	if spin := d.scene.Find(d.scene.Root(), "Spinner"); !spin.IsZero() {
		d.scene.SetRotation(spin, d.time)
	}
}

func (d *Demo) Render(float64) {
	if d.runner != nil {
		m := d.Engine.Metrics()
		d.runner.Prepare(m.PixelW, m.PixelH, m.Density())
		d.runner.Extract(d.Renderer)
	}
	d.BeginUI()
	d.DrawOverlay(func(c *core.Context) {
		if d.err != "" {
			widgets.Label(c, "load failed (F5 to retry)")
			widgets.LabelDim(c, d.err)
			return
		}
		st := d.runner.Stats()
		widgets.LabelDim(c, "F5 reload, F2 inspector")
		widgets.Labelf(c, "views %d sprites %d missing %d", st.Views, st.Sprites, st.MissingAssets)
		if d.show && d.panel != nil {
			d.panel.Draw(c)
		}
	})
	d.FlushUI()
}
