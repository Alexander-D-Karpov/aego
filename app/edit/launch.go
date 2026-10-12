package edit

import (
	"bytes"
	"errors"
	"os"
	"time"

	"aego/app/projects"
	"aego/core/log"
	"aego/core/mathx"
	editorapp "aego/editor/app"
	"aego/editor/launcher"
	"aego/engine"
	"aego/platform"
	"aego/render"
	"aego/render/icons"
	rendermodule "aego/render/module"
	"aego/render/text"
	"aego/render/view"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
)

type LaunchSession struct {
	Engine *engine.Engine
	UI     *launcher.Launcher
	driver *launchDriver
}

type launchDriver struct {
	s        *LaunchSession
	eng      *engine.Engine
	renderer *render.Renderer
	ui       *core.Context
	list     *draw.List
	textOn   bool
	log      *log.ChannelLogger
}

func CreateProject(dir, template, module, example string) (string, error) {
	opt := projects.Options{Dir: dir, Template: template, Module: module, Example: example}
	if example != "" {
		root, err := projects.EngineRoot("")
		if err != nil {
			return "", err
		}
		opt.Engine = root
	}
	p, err := projects.Create(opt)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := projects.Verify(p, &out); err != nil {
		return p.Root, errors.New(err.Error() + "\n" + out.String())
	}
	return p.Root, nil
}

func launcherTemplates() []launcher.Template {
	out := make([]launcher.Template, len(projects.Templates))
	for i, t := range projects.Templates {
		out[i] = launcher.Template{ID: t.ID, Name: t.Name, Description: t.Description, Icon: t.Icon}
	}
	return out
}

func OpenLauncher(cfg Config) (*LaunchSession, error) {
	lg := cfg.Logger
	if lg == nil {
		lg = log.New(log.NewTextSink(os.Stderr))
	}
	mod := rendermodule.New(render.Options{MaxSprites: 4096})
	e, err := engine.NewBuilder(engine.Config{
		Title:    "Aego",
		Width:    1100,
		Height:   760,
		Headless: cfg.Headless,
		Hidden:   cfg.Hidden,
		Debug:    cfg.Debug,
		AppID:    "aego.launcher",
	}).WithLogger(lg).Use(mod).Build()
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	var examples []string
	if root, err := projects.EngineRoot(""); err == nil {
		examples = projects.Examples(root)
	}
	ls := &LaunchSession{Engine: e}
	ls.UI = launcher.New(launcher.Options{
		Templates:  launcherTemplates(),
		Examples:   examples,
		Create:     CreateProject,
		RecentPath: cfg.Recent,
		Home:       home,
	})
	ls.driver = &launchDriver{s: ls, renderer: mod.Renderer(), log: lg.With(editorapp.ChannelEditor)}
	return ls, nil
}

func (ls *LaunchSession) Step() bool {
	return ls.Engine.Step(ls.driver)
}

func (ls *LaunchSession) Close() {
	ls.Engine.Close()
}

func Launch(cfg Config) (string, error) {
	ls, err := OpenLauncher(cfg)
	if err != nil {
		return "", err
	}
	if cfg.Frames > 0 {
		for i := 0; i < cfg.Frames && ls.Step(); i++ {
		}
	} else if err := ls.Engine.Run(ls.driver); err != nil {
		ls.Close()
		return "", err
	}
	ls.Close()
	root, _ := ls.UI.Result()
	return root, nil
}

func touchRecent(cfg Config, root, name string, lg *log.ChannelLogger) {
	if cfg.Recent == "" {
		return
	}
	if err := launcher.Touch(cfg.Recent, root, name, time.Now()); err != nil {
		lg.Warn("cannot update the recent projects list", log.F("err", err.Error()))
	}
}

func (d *launchDriver) Init(e *engine.Engine) error {
	d.eng = e
	r := d.renderer
	ui := core.New(r.UIFont())
	ui.Theme = theme.Editor()
	if set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetFonts(set)
	} else {
		d.log.Warn("ui fonts are missing, using the debug font", log.F("err", err.Error()))
	}
	if set, err := icons.Load(e.VFS(), r.Textures(), "engine://icons"); err == nil {
		ui.Icons = set
	}
	d.ui = ui
	return nil
}

func (d *launchDriver) Event(ev platform.Event) {
	switch ev.Kind {
	case platform.EvTextEditing:
		d.ui.SetComposition(ev.Str, int(ev.EditStart))
	case platform.EvTextInput:
		d.ui.SetComposition("", 0)
	}
}

func (d *launchDriver) FixedUpdate(float64) {}

func (d *launchDriver) Update(float64) {
	e, ui := d.eng, d.ui
	ui.Time = e.Platform().Now()
	m := e.Metrics()
	size := mathx.Vec2{X: float32(m.LogicalW), Y: float32(m.LogicalH)}
	ui.Begin(e.Input(), size, m.Density())
	d.s.UI.Draw(ui, mathx.Rect{W: size.X, H: size.Y})
	d.list = ui.End()
	d.applyPlatform()
	if _, ok := d.s.UI.Result(); ok {
		e.Quit()
	}
}

func (d *launchDriver) applyPlatform() {
	win := d.eng.Window()
	if cs, ok := win.(platform.Cursorer); ok {
		cs.SetCursor(platform.CursorShape(d.ui.CursorShape()))
	}
	ti, ok := win.(platform.TextInputer)
	if !ok {
		return
	}
	wants, r, cursor := d.ui.WantsTextInput()
	switch {
	case wants && !d.textOn:
		ti.StartTextInput()
		d.textOn = true
	case !wants && d.textOn:
		ti.StopTextInput()
		d.textOn = false
		d.ui.SetComposition("", 0)
	}
	if wants {
		ti.SetTextInputArea(r, cursor)
	}
}

func (d *launchDriver) Render(float64) {
	if d.list == nil || d.list.Empty() {
		return
	}
	m := d.eng.Metrics()
	v := view.ScreenView(m.PixelW, m.PixelH, m.Density())
	v.Clear = d.ui.Theme.Background
	v.DoClear = true
	v.Label = "launcher.ui"
	white := d.renderer.White()
	if f := d.ui.Font(); f != nil && !f.White.IsZero() {
		white = f.White
	}
	f := d.renderer.View(v)
	d.list.Flush(f, d.ui.Font(), white, 0)
	f.End()
}

func (d *launchDriver) Shutdown() {}
