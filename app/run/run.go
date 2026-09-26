package run

import (
	"aego/asset/importers/atlas"
	"aego/asset/importers/scenefile"
	texImporter "aego/asset/importers/texture"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/engine"
	"aego/project"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
	"aego/scene/systems"
	"aego/schema"
	"aego/sdk"
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"time"
)

type Config struct {
	Headless   bool
	Frames     int
	Hidden     bool
	Debug      bool
	Watch      bool
	Supervised bool
	SceneID    ids.AssetID
	ScenePath  string
	Logger     *log.Logger
}

type App struct {
	Project  *project.Project
	Engine   *engine.Engine
	Scene    *scene.Scene
	Pipeline *pipeline.Pipeline
	Loader   *loader.Loader
	Runner   *systems.Runner
	Renderer *render.Renderer

	cfg      Config
	log      *log.ChannelLogger
	stopW    func()
	modules  []sdk.Module
	registry *sdk.Registry
	types    *scene.Types
	sceneID  ids.AssetID
	driver   *sceneApp
}

func Main(mods ...sdk.Module) {
	fs := flag.NewFlagSet("game", flag.ExitOnError)
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	scenePath := fs.String("scene", "", "load this scene file instead of the main scene")
	supervised := fs.Bool("supervised", false, "accept restart commands on stdin")
	fs.Parse(os.Args[1:])

	root, err := project.Find(".")
	if err != nil {
		fail(err)
	}
	p, err := project.Load(root)
	if err != nil {
		fail(err)
	}
	cfg := Config{
		Headless:   *headless,
		Frames:     *frames,
		Debug:      *debug,
		Watch:      *frames == 0,
		Supervised: *supervised,
		ScenePath:  *scenePath,
	}
	if err := Run(p, cfg, mods...); err != nil {
		fail(err)
	}
}

func fail(err error) {
	os.Stderr.WriteString("aego: " + err.Error() + "\n")
	os.Exit(1)
}

func DefaultImporters() (*pipeline.ImporterRegistry, error) {
	reg := pipeline.NewImporterRegistry()
	if err := reg.Register(atlas.Wrap(texImporter.New())); err != nil {
		return nil, err
	}
	if err := reg.Register(scenefile.NewScene()); err != nil {
		return nil, err
	}
	if err := reg.Register(scenefile.NewPrefab()); err != nil {
		return nil, err
	}
	return reg, nil
}

func Open(p *project.Project, cfg Config, mods ...sdk.Module) (*App, error) {
	lg := cfg.Logger
	if lg == nil {
		lg = log.New(log.NewTextSink(os.Stderr))
		if cfg.Debug {
			lg.SetLevel(log.Debug)
		}
	}
	if cfg.ScenePath != "" && !filepath.IsAbs(cfg.ScenePath) {
		if _, err := os.Stat(cfg.ScenePath); err != nil {
			cfg.ScenePath = filepath.Join(p.Root, cfg.ScenePath)
		}
	}
	app := &App{Project: p, cfg: cfg, log: lg.With(log.Engine), modules: mods}

	imps, err := DefaultImporters()
	if err != nil {
		return nil, err
	}
	app.Pipeline, err = pipeline.Open(p, imps, lg)
	if err != nil {
		return nil, err
	}
	if _, _, err := app.Pipeline.Scan(); err != nil {
		return nil, err
	}
	batch, err := app.Pipeline.ImportAll()
	if err != nil {
		return nil, err
	}
	for id, ierr := range batch.Failed {
		app.log.Error("asset import failed", log.F("asset", id.String()), log.F("err", ierr.Error()))
	}

	rmod := rendermodule.New(render.Options{MaxSprites: 65536})
	builder := engine.NewBuilder(engine.Config{
		Title:    p.Window.Title,
		Width:    p.Window.Width,
		Height:   p.Window.Height,
		Headless: cfg.Headless,
		Hidden:   cfg.Hidden,
		Debug:    cfg.Debug,
		AppID:    p.Module,
		ResRoot:  p.AssetsDir(),
	}).Use(rmod)

	app.types = newTypes()
	app.registry = &sdk.Registry{
		Engine:    builder.Registry(),
		Types:     app.types,
		Importers: imps,
	}

	app.Runner = systems.New()
	if err := systems.Register(builder.Registry(), app.Runner); err != nil {
		return nil, err
	}
	for _, m := range mods {
		if err := m.Register(app.registry); err != nil {
			return nil, errs.Wrap(err, errs.EngineModule, "user module registration failed").
				With(log.F("module", m.Info().ID))
		}
	}
	if err := app.types.Schema().Freeze(); err != nil {
		return nil, err
	}

	eng, err := builder.Build()
	if err != nil {
		return nil, err
	}
	app.Engine = eng
	app.Renderer = rmod.Renderer()

	app.Loader = loader.New(eng.GPU(), app.Renderer.Textures(), lg)
	if err := app.Loader.Mount(app.Pipeline.Index(), app.Pipeline.Artifacts()); err != nil {
		return nil, err
	}
	eng.SetService(loader.ServiceID, app.Loader)
	eng.SetService(rendermodule.ServiceID, app.Renderer)

	sceneID := cfg.SceneID
	if sceneID.IsZero() {
		sceneID = p.MainScene
	}
	if err := app.loadScene(app.types, sceneID, cfg.ScenePath); err != nil {
		return nil, err
	}
	app.sceneID = sceneID
	eng.SetService(scene.ServiceID, app.Scene)
	app.Runner.Bind(app.Scene, app.Renderer, app.Loader)
	return app, nil
}

func newTypes() *scene.Types {
	t := scene.NewTypes(schema.NewRegistry())
	if err := components.Register(t); err != nil {
		panic("aego: builtin components failed to register: " + err.Error())
	}
	if err := sceneio.RegisterUnknown(t); err != nil {
		panic("aego: unknown component holder failed to register: " + err.Error())
	}
	return t
}

func (a *App) loadScene(types *scene.Types, id ids.AssetID, path string) error {
	var src []byte
	name := "main.aescene"
	switch {
	case path != "":
		data, err := os.ReadFile(path)
		if err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot read the scene file")
		}
		src, name = data, path
	case !id.IsZero():
		data, ok := a.Loader.Scene(id)
		if !ok {
			return errs.New(errs.AssetNotFound, "main scene is not in the asset index",
				log.F("asset", id.String()))
		}
		src = data
	default:
		a.Scene = scene.New(types, a.Loader, scene.Options{})
		return nil
	}
	s, report, err := sceneio.Load(src, name, types, a.Loader, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return err
	}
	for _, w := range report.Warnings {
		a.log.Warn("scene warning", log.F("path", w.Path), log.F("line", w.Pos.Line), log.F("msg", w.Msg))
	}
	for _, u := range report.Unknown {
		a.log.Warn("unknown component preserved", log.F("type", u))
	}
	a.Scene = s
	return nil
}

func (a *App) StartWatch() error {
	if !a.cfg.Watch {
		return nil
	}
	stop, err := a.Pipeline.Watch(150*time.Millisecond, func(b *pipeline.Batch) {
		a.Engine.Jobs().OnMain(func() { a.applyBatch(b) })
	})
	if err != nil {
		return err
	}
	a.stopW = stop
	return nil
}

func (a *App) applyBatch(b *pipeline.Batch) {
	if len(b.Imported) == 0 {
		return
	}
	if err := a.Loader.Swap(b.Imported, a.Pipeline.Artifacts()); err != nil {
		a.log.Error("hot reload failed, keeping the previous assets", log.F("err", err.Error()))
		return
	}
	a.log.Info("assets reloaded", log.F("count", len(b.Imported)))
}

func (a *App) Run() error {
	defer a.Close()
	if err := a.StartWatch(); err != nil {
		return err
	}
	if a.cfg.Supervised {
		a.superviseStdin()
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	go func() {
		if _, ok := <-interrupts; ok {
			a.Engine.Jobs().OnMain(a.Engine.Quit)
		}
	}()
	driver := a.Driver()
	if a.cfg.Frames > 0 {
		for i := 0; i < a.cfg.Frames; i++ {
			if !a.Engine.Step(driver) {
				break
			}
		}
		return a.Engine.Err()
	}
	return a.Engine.Run(driver)
}

func (a *App) Driver() *sceneApp {
	if a.driver == nil {
		a.driver = &sceneApp{app: a}
	}
	return a.driver
}

func (a *App) Close() {
	if a.stopW != nil {
		a.stopW()
		a.stopW = nil
	}
	if a.Loader != nil {
		a.Loader.Close()
	}
	if a.Pipeline != nil {
		a.Pipeline.Close()
	}
}

func Run(p *project.Project, cfg Config, mods ...sdk.Module) error {
	app, err := Open(p, cfg, mods...)
	if err != nil {
		return err
	}
	return app.Run()
}
