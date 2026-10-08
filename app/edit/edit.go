package edit

import (
	"aego/asset"
	"aego/script/abi"
	scriptmodule "aego/script/module"
	"aego/script/stdlib"
	"errors"
	"flag"
	"os"
	"os/signal"
	"runtime/pprof"
	"time"

	"aego/app/run"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/log"
	editorapp "aego/editor/app"
	"aego/editor/extension"
	"aego/editor/session"
	"aego/engine"
	"aego/project"
	"aego/render"
	rendermodule "aego/render/module"
	"aego/scene/systems"
	"aego/sdk"
)

var ErrRestart = errors.New("editor restart requested")

type Config struct {
	Headless   bool
	Hidden     bool
	Debug      bool
	Frames     int
	Width      int
	Height     int
	NoWatch    bool
	Supervised bool
	CPUProfile string
	Logger     *log.Logger
}

type Session struct {
	Project  *project.Project
	Engine   *engine.Engine
	Pipeline *pipeline.Pipeline
	Loader   *loader.Loader
	App      *editorapp.App
	Scripts  *scriptHost
	cfg      Config
	log      *log.ChannelLogger
	stop     func()
	unsub    func()
}

func Main(mods ...sdk.Module) {
	fs := flag.NewFlagSet("aego-editor", flag.ExitOnError)
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	hidden := fs.Bool("hidden", false, "create the window without showing it")
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	cpu := fs.String("cpuprofile", "", "write a cpu profile to this file")
	fs.Parse(os.Args[1:])
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	root, err := project.Find(dir)
	if err != nil {
		fail(err)
	}
	p, err := project.Load(root)
	if err != nil {
		fail(err)
	}
	cfg := Config{
		Debug:      *debug,
		Hidden:     *hidden,
		Headless:   *headless,
		Frames:     *frames,
		CPUProfile: *cpu,
		Supervised: os.Getenv(session.EnvSupervised) == "1",
	}
	err = Run(p, cfg, mods...)
	if errors.Is(err, ErrRestart) {
		os.Exit(session.ExitRestart)
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	os.Stderr.WriteString("aego-editor: " + err.Error() + "\n")
	os.Exit(1)
}

func Run(p *project.Project, cfg Config, mods ...sdk.Module) error {
	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			return err
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			return err
		}
		defer func() {
			pprof.StopCPUProfile()
			f.Close()
		}()
	}
	s, err := Open(p, cfg, mods...)
	if err != nil {
		return err
	}
	if err := s.Run(); err != nil {
		return err
	}
	if s.App.Restarting() {
		return ErrRestart
	}
	return nil
}

func Open(p *project.Project, cfg Config, mods ...sdk.Module) (*Session, error) {
	lg := cfg.Logger
	if lg == nil {
		lg = log.New(log.NewTextSink(os.Stderr))
		if cfg.Debug {
			lg.SetLevel(log.Debug)
		}
	}
	if cfg.Width <= 0 {
		cfg.Width = 1600
	}
	if cfg.Height <= 0 {
		cfg.Height = 900
	}
	s := &Session{Project: p, cfg: cfg, log: lg.With(editorapp.ChannelEditor)}

	var surface abi.Surface
	imps, err := run.Importers(p.Module, func() abi.Surface { return surface })
	if err != nil {
		return nil, err
	}
	rmod := rendermodule.New(render.Options{MaxSprites: 65536})
	builder := engine.NewBuilder(engine.Config{
		Title:     p.Name + " — Aego Editor",
		Width:     cfg.Width,
		Height:    cfg.Height,
		Headless:  cfg.Headless,
		Hidden:    cfg.Hidden,
		Debug:     cfg.Debug,
		AppID:     p.Module + ".editor",
		ResRoot:   p.AssetsDir(),
		LoopScope: engine.ScopeFrame,
	}).WithLogger(lg).Use(rmod).Use(scriptmodule.New())

	types := run.NewTypes()
	bindings := scriptmodule.Bindings(stdlib.Options{Input: &p.Input})
	reg := &sdk.Registry{Engine: builder.Registry(), Types: types, Importers: imps, Script: bindings}
	if err := systems.Register(builder.Registry(), systems.New()); err != nil {
		return nil, err
	}
	for _, m := range mods {
		if err := m.Register(reg); err != nil {
			return nil, errs.Wrap(err, errs.EngineModule, "user module registration failed").
				With(log.F("module", m.Info().ID))
		}
	}
	if err := types.Schema().Freeze(); err != nil {
		return nil, err
	}
	if err := scriptmodule.Freeze(bindings, types.Schema()); err != nil {
		return nil, errs.Wrap(err, errs.EngineModule, "script bindings are invalid")
	}
	surface = bindings.Surface()

	s.Pipeline, err = pipeline.Open(p, imps, lg)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.Pipeline.Scan(); err != nil {
		s.Pipeline.Close()
		return nil, err
	}
	batch, err := s.Pipeline.ImportAll()
	if err != nil {
		s.Pipeline.Close()
		return nil, err
	}
	for id, ierr := range batch.Failed {
		s.log.Error("asset import failed", log.F("asset", id.String()), log.F("err", ierr.Error()))
	}

	s.Engine, err = builder.Build()
	if err != nil {
		s.Pipeline.Close()
		return nil, err
	}
	s.Loader = loader.New(s.Engine.GPU(), rmod.Renderer().Textures(), lg)
	if err := s.Loader.Mount(s.Pipeline.Index(), s.Pipeline.Artifacts()); err != nil {
		s.Close()
		return nil, err
	}
	s.Engine.SetService(loader.ServiceID, s.Loader)
	s.Engine.SetService(rendermodule.ServiceID, rmod.Renderer())

	s.Scripts = &scriptHost{
		bindings: bindings,
		schema:   types.Schema(),
		loader:   s.Loader,
		pipeline: s.Pipeline,
		importer: run.ScriptImporter(imps),
		log:      lg,
	}
	if err := s.Scripts.Reload(); err != nil {
		s.log.Error("cannot publish script types", log.F("err", err.Error()))
	}
	s.unsub = s.Loader.Subscribe(func(r asset.Reloaded) {
		if r.Kind != asset.ArtScript {
			return
		}
		if err := s.Scripts.Reload(); err != nil {
			s.log.Error("cannot publish script types", log.F("err", err.Error()))
		}
	})

	var exts []extension.Module
	for _, m := range mods {
		if em, ok := m.(extension.Module); ok {
			exts = append(exts, em)
		}
	}
	s.App = editorapp.New(editorapp.Options{
		Project:     p,
		Pipeline:    s.Pipeline,
		Loader:      s.Loader,
		Types:       types,
		Renderer:    rmod.Renderer(),
		Log:         lg,
		Extensions:  exts,
		RenderStats: rmod.Stats,
		Supervised:  cfg.Supervised,
		Paced:       !cfg.Headless,
		Scripts:     s.Scripts,
	})
	return s, nil
}

func (s *Session) applyBatch(b *pipeline.Batch) {
	if ctx := s.App.Context(); ctx != nil {
		ctx.ApplyBatch(b)
		return
	}
	if len(b.Imported) == 0 {
		return
	}
	if err := s.Loader.Swap(b.Imported, s.Pipeline.Artifacts()); err != nil {
		s.log.Error("hot reload failed, keeping the previous assets", log.F("err", err.Error()))
	}
}

func (s *Session) Step() bool {
	return s.Engine.Step(s.App)
}

func (s *Session) Run() error {
	defer s.Close()
	if !s.cfg.NoWatch && s.cfg.Frames == 0 {
		stop, err := s.Pipeline.Watch(150*time.Millisecond, func(b *pipeline.Batch) {
			s.Engine.Jobs().OnMain(func() { s.applyBatch(b) })
		})
		if err != nil {
			return err
		}
		s.stop = stop
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	go func() {
		if _, ok := <-interrupts; ok {
			s.Engine.Jobs().OnMain(s.Engine.Quit)
		}
	}()
	if s.cfg.Frames > 0 {
		for i := 0; i < s.cfg.Frames; i++ {
			if !s.Engine.Step(s.App) {
				break
			}
		}
		return s.Engine.Close()
	}
	return s.Engine.Run(s.App)
}

func (s *Session) Close() {
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	if s.Engine != nil {
		s.Engine.Close()
	}
	if s.Loader != nil {
		s.Loader.Close()
		s.Loader = nil
	}
	if s.Pipeline != nil {
		s.Pipeline.Close()
		s.Pipeline = nil
	}
}
