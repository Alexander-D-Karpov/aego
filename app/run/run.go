package run

import (
	"flag"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"time"

	"aego/app"
	"aego/app/crash"
	"aego/asset/pipeline"
	"aego/contracts/persistence"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/rng"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/recorder"
	"aego/project"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/script/reload"
	scriptruntime "aego/script/runtime"
	"aego/sdk"
)

type Config struct {
	Headless   bool
	FrameTime  float64
	Frames     int
	Hidden     bool
	Debug      bool
	Watch      bool
	Supervised bool
	SceneID    ids.AssetID
	ScenePath  string
	Trace      string
	Profile    string
	Logger     *log.Logger
	Seed       uint64
	NoProfile  bool
	// SharedIndex is a run the editor or the CLI starts on a project it already has open.
	SharedIndex bool
}

type App struct {
	*app.Host
	Scene   *scene.Scene
	Runner  *frame.Runner
	Scripts *scriptruntime.Runtime

	cfg     Config
	log     *log.ChannelLogger
	stopW   func()
	detach  func()
	sceneID ids.AssetID
	driver  *sceneApp
	prog    reload.Program
	target  uint64
	unrec   func()

	ring       *crash.Ring
	crashScene bool
}

func Main(mods ...sdk.Module) {
	if len(os.Args) > 1 && os.Args[1] == "--tool" {
		root, err := project.Find(".")
		if err != nil {
			fail(err)
		}
		p, err := project.Load(root)
		if err != nil {
			fail(err)
		}
		os.Exit(Tool(p, os.Args[2:], os.Stdout, os.Stderr, mods...))
	}
	fs := flag.NewFlagSet("game", flag.ExitOnError)
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	scenePath := fs.String("scene", "", "load this scene file instead of the main scene")
	supervised := fs.Bool("supervised", false, "accept restart commands on stdin")
	tracePath := fs.String("trace", "", "write a Chrome trace of the last 239 frames to this file on exit")
	profile := fs.String("profile", "", "player settings profile, default when empty")
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
		Trace:      *tracePath,
		Profile:    *profile,
	}
	if err := Run(p, cfg, mods...); err != nil {
		fail(err)
	}
}

func fail(err error) {
	os.Stderr.WriteString("aego: " + err.Error() + "\n")
	os.Exit(1)
}

func Open(p *project.Project, cfg Config, mods ...sdk.Module) (*App, error) {
	if cfg.ScenePath != "" && !filepath.IsAbs(cfg.ScenePath) {
		if _, err := os.Stat(cfg.ScenePath); err != nil {
			cfg.ScenePath = filepath.Join(p.Root, cfg.ScenePath)
		}
	}
	h, err := app.Build(p, app.Config{
		Engine: engine.Config{
			Title:             p.Window.Title,
			Width:             p.Window.Width,
			Height:            p.Window.Height,
			Headless:          cfg.Headless,
			HeadlessFrameTime: cfg.FrameTime,
			Hidden:            cfg.Hidden,
			FixedFrameTime:    hiddenFrame(cfg),
			Debug:             cfg.Debug,
			AppID:             p.Module,
			Trace:             cfg.Trace != "",
			Seed:              cfg.Seed,
		},
		Logger:             cfg.Logger,
		Debug:              cfg.Debug,
		FailOnScriptErrors: true,
		Profile:            cfg.Profile,
		NoProfile:          cfg.NoProfile,
		SharedIndex:        cfg.SharedIndex,
	}, sdk.AdaptAll(mods)...)
	if err != nil {
		return nil, err
	}
	a := &App{Host: h, cfg: cfg, log: h.Log.With(log.Engine), Runner: frame.New()}
	a.ring = crash.NewRing(500)
	h.Log.AddSink(a.ring)
	if st, ok := module.SectionValue[base.Settings](h.Sections, base.Section); ok {
		a.crashScene = st.CrashScene
	}
	h.Engine.Jobs().SetPanicHandler(func(rec any, stack []byte) { a.crash("panic", "worker", rec, stack, "") })
	if err := a.start(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *App) start() error {
	a.Engine.SetView(frame.ServiceID, a.Runner)
	a.Scripts = scriptruntime.New(scriptruntime.Options{Bindings: a.Bindings, Schema: a.Types.Schema(), Log: a.Log, OnFault: a.fault, OnPanic: a.scriptPanic})
	if err := a.linkScripts(); err != nil {
		return err
	}
	a.Engine.SetService(scriptruntime.ServiceID, a.Scripts)
	a.Engine.SetView(persistence.HostID, a)
	a.sceneID = a.cfg.SceneID
	if a.sceneID.IsZero() {
		a.sceneID = a.Project.MainScene
	}
	if err := a.loadScene(a.Types, a.sceneID, a.cfg.ScenePath); err != nil {
		return err
	}
	a.Engine.SetService(scene.ServiceID, a.Scene)
	w := a.Engine.MainWorld()
	w.Recorder = recorder.New(4096)
	if a.cfg.Seed != 0 {
		w.Random = rng.New(a.cfg.Seed)
	}
	a.unrec = frame.Record(a.Scene, w.Recorder)
	a.Runner.Bind(a.Scene, a.Renderer, a.Loader)
	if err := a.Scripts.Instantiate(a.Scene); err != nil {
		return err
	}
	return a.attach()
}

func (a *App) attach() error {
	if a.detach != nil {
		a.detach()
		a.detach = nil
	}
	d, err := a.Modules.Attach(&module.AttachContext{World: a.Engine.MainWorld(), Scene: a.Scene, Log: a.Engine.Log(), Play: true})
	if err != nil {
		return err
	}
	a.detach = d
	return nil
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
	for id := range b.Failed {
		a.Loader.RejectSource(id)
	}
	if len(b.Imported) > 0 {
		if err := a.Loader.Swap(b.Imported, a.Pipeline.Artifacts()); err != nil {
			a.log.Error("hot reload failed, keeping the previous assets", log.F("err", err.Error()))
			return
		}
	}
	for _, id := range b.Removed {
		a.Loader.Drop(id)
	}
	if len(b.Imported) > 0 {
		a.log.Info("assets reloaded", log.F("count", len(b.Imported)))
	}
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
	if a.detach != nil {
		a.detach()
		a.detach = nil
	}
	if a.Scripts != nil {
		a.Scripts.Close()
	}
	if a.Runner != nil {
		a.Runner.Close()
	}
	if a.cfg.Trace != "" && a.Engine != nil {
		if err := WriteTrace(a.cfg.Trace, a.Engine.Trace()); err != nil {
			a.log.Error("cannot write the trace", log.F("path", a.cfg.Trace), log.F("err", err.Error()))
		}
	}
	a.Host.Close()
}

func Run(p *project.Project, cfg Config, mods ...sdk.Module) error {
	a, err := Open(p, cfg, mods...)
	if err != nil {
		return err
	}
	defer func() {
		if rec := recover(); rec != nil {
			a.crash("panic", "run", rec, debug.Stack(), "")
			panic(rec)
		}
	}()
	return a.Run()
}

// hiddenFrame is the fixed frame time of a hidden windowed run, which renders for screenshots but must step like a headless one.
func hiddenFrame(cfg Config) float64 {
	if cfg.Hidden && !cfg.Headless {
		return cfg.FrameTime
	}
	return 0
}
