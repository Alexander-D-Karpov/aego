package engine

import (
	"image"
	"os"
	"runtime"
	"runtime/metrics"

	"aego/core/clock"
	"aego/core/errs"
	"aego/core/log"
	"aego/core/rng"
	"aego/core/stats"
	"aego/core/trace"
	"aego/engine/res"
	"aego/gpu"
	"aego/gpu/null"
	"aego/platform"
	"aego/platform/headless"
	"aego/platform/input"
	"aego/vfs"
)

type Engine struct {
	cfg        Config
	log        *log.Logger
	elog       *log.ChannelLogger
	plat       platform.Platform
	clockReset platform.FrameClockResetter
	win        platform.Window
	dev        gpu.Device
	capture    func(*image.RGBA, error)
	curFrame   gpu.Frame
	files      *vfs.VFS
	input      input.State
	reg        *Registry
	jobs       *Jobs
	counter    *stats.Counter
	stats      stats.Frame
	metrics    platform.WindowMetrics
	ctx        Context
	main       *World
	wctx       Context
	app        App
	framePacer FramePacer
	onEvent    func(platform.Event)
	vsync      platform.VSyncMode
	frameLimit float64
	frameNo    uint64
	lastTime   float64
	minimized  bool
	appInit    bool
	quit       bool
	closed     bool
	closers    []func()
	err        error

	probe       bool
	probeSample []metrics.Sample
	allocs      [phaseCount]uint64
	lastAllocs  [phaseCount]uint64
	allocTotal  [allocZones]uint64
	lockstep    bool
	frameBytes  uint64

	tracer     *trace.Tracer
	phaseZones [phaseCount]trace.ZoneID
	zPoll      trace.ZoneID
	zSwap      trace.ZoneID
}

type Builder struct {
	cfg      Config
	reg      *Registry
	eng      *Engine
	logger   *log.Logger
	platform platform.Platform
	mods     []Module
	built    bool
	err      error
}

func NewBuilder(cfg Config) *Builder {
	b := &Builder{cfg: cfg, reg: newRegistry(), eng: &Engine{main: NewWorld("main")}}
	sys := b.reg.Systems()
	_ = sys.Register(SystemDesc{ID: SystemAppFixed, Phase: PhasePhysics, Scope: ScopeFrame, System: &appSystem{e: b.eng, kind: appFixed}})
	_ = sys.Register(SystemDesc{ID: SystemAppUpdate, Phase: PhaseUpdate, Scope: ScopeFrame, System: &appSystem{e: b.eng, kind: appUpdate}})
	_ = sys.Register(SystemDesc{ID: SystemAppRender, Phase: PhaseRender, Scope: ScopeFrame, System: &appSystem{e: b.eng, kind: appRender}})
	return b
}

func (b *Builder) Use(m Module) *Builder {
	if b.err != nil {
		return b
	}
	info := m.Info()
	for _, existing := range b.reg.modules {
		if existing.ID == info.ID {
			b.err = errs.New(errs.EngineModule, "duplicate module id", log.F("module", info.ID))
			return b
		}
	}
	if err := m.Register(b.reg); err != nil {
		b.err = errs.Wrap(err, errs.EngineModule, "module registration failed").With(log.F("module", info.ID))
		return b
	}
	b.reg.modules = append(b.reg.modules, info)
	b.mods = append(b.mods, m)
	return b
}

func (b *Builder) WithLogger(l *log.Logger) *Builder {
	b.logger = l
	return b
}

func (b *Builder) WithPlatform(p platform.Platform) *Builder {
	b.platform = p
	return b
}

func (b *Builder) Configure(fn func(*Config)) *Builder {
	if fn != nil {
		fn(&b.cfg)
	}
	return b
}

func (b *Builder) Registry() *Registry {
	return b.reg
}

func (e *Engine) TimeConfig() clock.Config {
	return clock.Config{FixedDelta: 1 / e.cfg.FixedHz, MaxFrameDelta: e.cfg.MaxFrameDelta, MaxFixedSteps: e.cfg.MaxFixedSteps}
}

func (b *Builder) Build() (*Engine, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.built {
		return nil, errs.New(errs.EngineConfig, "builder has already produced an engine")
	}
	b.built = true

	cfg := b.cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	logger := b.logger
	if logger == nil {
		logger = log.New(log.NewTextSink(os.Stderr))
		logger.SetLevel(cfg.LogLevel)
	}
	if err := b.reg.Freeze(); err != nil {
		return nil, err
	}

	e := b.eng
	e.cfg = cfg
	e.log = logger
	e.elog = logger.With(log.Engine)
	e.reg = b.reg
	e.initTrace()
	e.ctx.Engine = e
	e.ctx.World = e.main
	e.onEvent = e.handleEvent
	e.counter = stats.NewCounter()
	e.main.Time.Configure(e.TimeConfig())

	if err := e.initBackends(b.platform); err != nil {
		e.Close()
		return nil, err
	}
	files, err := vfs.New(
		vfs.Mount{Scheme: "res", Root: cfg.ResRoot, ReadOnly: true},
		vfs.Mount{Scheme: "user", Root: cfg.userRoot()},
		vfs.Mount{Scheme: "engine", Source: res.Source(), ReadOnly: true},
	)
	if err != nil {
		e.Close()
		return nil, err
	}
	e.files = files
	e.jobs = newJobs(cfg.Workers)
	e.ctx.Random = rng.New(cfg.Seed)
	e.logStartup()
	if err := b.initModules(e); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

func (e *Engine) initTrace() {
	e.tracer = trace.New(trace.Options{})
	for p := range e.reg.systems.phases {
		e.phaseZones[p] = e.tracer.Zone("phase." + Phase(p).String())
		for i := range e.reg.systems.phases[p] {
			s := &e.reg.systems.phases[p][i]
			s.zone = e.tracer.Zone(s.id)
		}
	}
	e.zPoll = e.tracer.Zone("engine.poll")
	e.zSwap = e.tracer.Zone("engine.swap")
	e.tracer.SetEnabled(e.cfg.Trace)
}

func (e *Engine) Trace() *trace.Tracer {
	return e.tracer
}

func (b *Builder) initModules(e *Engine) error {
	for i, m := range b.mods {
		init, ok := m.(ModuleIniter)
		if !ok {
			continue
		}
		if err := init.Init(e); err != nil {
			return errs.Wrap(err, errs.EngineModule, "module initialization failed").
				With(log.F("module", b.reg.modules[i].ID))
		}
	}
	return nil
}

func (e *Engine) initBackends(p platform.Platform) error {
	cfg := e.cfg
	if p == nil {
		if cfg.Headless {
			p = headless.New(headless.Options{FrameTime: cfg.HeadlessFrameTime})
		} else {
			np, err := newNativePlatform()
			if err != nil {
				return err
			}
			p = np
		}
	}
	e.plat = p
	// Resolve the optional capability once per backend. A failed type assertion
	// on every frame can lazily allocate a Go runtime cache after warmup.
	e.clockReset, _ = p.(platform.FrameClockResetter)

	win, err := p.CreateWindow(platform.WindowDesc{
		Title:      cfg.Title,
		Width:      cfg.Width,
		Height:     cfg.Height,
		Resizable:  !cfg.FixedSize,
		Fullscreen: cfg.Fullscreen,
		HighDPI:    true,
		SRGB:       true,
		Debug:      cfg.Debug,
		Hidden:     cfg.Hidden,
		VSync:      cfg.VSync,
	})
	if err != nil {
		return err
	}
	e.win = win
	e.metrics = win.Metrics()
	e.vsync = win.SetVSync(cfg.VSync)

	if !cfg.Headless {
		dev, err := newNativeDevice(win, cfg, e.log)
		if err != nil {
			return err
		}
		if dev != nil {
			e.dev = dev
			return nil
		}
		e.elog.Warn("window has no GPU context, falling back to the null gpu backend")
	}
	e.dev = null.New()
	return nil
}

func (e *Engine) logStartup() {
	info := e.plat.Info()
	caps := e.dev.Caps()
	m := e.metrics
	e.elog.Info("aego started",
		log.F("version", Version), log.F("go", runtime.Version()),
		log.F("os", info.OS), log.F("arch", info.Arch))
	e.elog.Info("platform",
		log.F("backend", info.Backend), log.F("version", info.Version),
		log.F("video_driver", info.VideoDriver), log.F("workers", e.jobs.Workers()))
	e.elog.Info("gpu",
		log.F("vendor", caps.Vendor), log.F("renderer", caps.Renderer),
		log.F("version", caps.Version), log.F("glsl", caps.GLSL),
		log.F("debug_output", caps.DebugOutput))
	e.elog.Info("window",
		log.F("logical_w", m.LogicalW), log.F("logical_h", m.LogicalH),
		log.F("pixel_w", m.PixelW), log.F("pixel_h", m.PixelH),
		log.F("density", m.PixelDensity), log.F("display_scale", m.DisplayScale))
	e.elog.Info("display",
		log.F("srgb_framebuffer", caps.SRGB),
		log.F("vsync_requested", e.cfg.VSync.String()),
		log.F("vsync_applied", e.vsync.String()),
		log.F("fixed_hz", e.cfg.FixedHz))
}

func (e *Engine) Platform() platform.Platform { return e.plat }
func (e *Engine) Window() platform.Window     { return e.win }
func (e *Engine) GPU() gpu.Device             { return e.dev }
func (e *Engine) Frame() gpu.Frame            { return e.curFrame }
func (e *Engine) Input() *input.State         { return &e.input }
func (e *Engine) VFS() *vfs.VFS               { return e.files }
func (e *Engine) Log() *log.Logger            { return e.log }
func (e *Engine) Stats() *stats.Frame         { return &e.stats }
func (e *Engine) Jobs() *Jobs                 { return e.jobs }
func (e *Engine) Caps() gpu.Capabilities      { return e.dev.Caps() }
func (e *Engine) Registry() *Registry         { return e.reg }
func (e *Engine) Config() Config              { return e.cfg }
func (e *Engine) Err() error                  { return e.err }
func (e *Engine) FrameNumber() uint64         { return e.frameNo }

func (e *Engine) Metrics() platform.WindowMetrics {
	return e.metrics
}

func (e *Engine) VSync() platform.VSyncMode {
	return e.vsync
}

func (e *Engine) SetVSync(m platform.VSyncMode) platform.VSyncMode {
	e.vsync = e.win.SetVSync(m)
	return e.vsync
}

func (e *Engine) Quit() {
	e.quit = true
}

func (e *Engine) CancelQuit() {
	e.quit = false
}

func (e *Engine) Run(app App) error {
	step := func() bool { return e.Step(app) }
	if d, ok := e.plat.(platform.Driver); ok {
		d.Drive(step)
	} else {
		for step() {
		}
	}
	return e.Close()
}

// OnClose runs fn in Close after the app shuts down and before the window and platform close, last registered first.
func (e *Engine) OnClose(fn func()) {
	if fn != nil {
		e.closers = append(e.closers, fn)
	}
}

func (e *Engine) Close() error {
	if e.closed {
		return e.err
	}
	e.closed = true
	if e.appInit && e.app != nil {
		e.app.Shutdown()
	}
	for i := len(e.closers) - 1; i >= 0; i-- {
		e.closers[i]()
	}
	e.closers = nil
	if e.jobs != nil {
		e.jobs.stop()
	}
	if e.dev != nil {
		e.dev.Close()
	}
	if e.win != nil {
		e.win.Close()
	}
	if e.plat != nil {
		e.plat.Shutdown()
	}
	return e.err
}

func (e *Engine) handleEvent(ev platform.Event) {
	switch ev.Kind {
	case platform.EvQuit:
		e.quit = true
	case platform.EvWindowResized:
		e.metrics = ev.Window
	case platform.EvWindowMinimized:
		e.minimized = true
	case platform.EvWindowRestored:
		e.minimized = false
		e.metrics = ev.Window
	}
	platform.ApplyInput(&e.input, ev)
	if e.app != nil {
		e.app.Event(ev)
	}
}

func (e *Engine) SetFrameLimit(fps float64) {
	e.frameLimit = max(fps, 0)
	if l, ok := e.plat.(platform.FrameLimiter); ok {
		l.SetFrameLimit(e.frameLimit)
	}
}

func (e *Engine) FrameLimit() float64 {
	return e.frameLimit
}
