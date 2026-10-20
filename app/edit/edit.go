package edit

import (
	"errors"
	"flag"
	"os"
	"os/signal"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"aego/app"
	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/assembly/editors"
	"aego/app/crash"
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/editor/core/document"
	"aego/editor/core/session"
	"aego/editor/panels/scripteditor"
	"aego/editor/shell"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/platform"
	"aego/project"
	"aego/scene"
	"aego/script/langsvc"
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
	Recent     string
	Profile    string
	Logger     *log.Logger
	Catalog    assembly.Catalog
}

type docAttach struct {
	id     document.ID
	scene  *scene.Scene
	detach func()
}

type driver struct {
	*shell.App
	s *Session
}

func (d *driver) Update(dt float64) {
	d.s.syncDocs()
	d.s.spike()
	d.App.Update(dt)
}

type Session struct {
	*app.Host
	App     *shell.App
	Scripts *scriptHost
	cfg     Config
	log     *log.ChannelLogger
	stop    func()
	unsub   func()
	docs    []docAttach
	driver  *driver

	ring       *crash.Ring
	crashScene bool
	spikeMs    float64
	lastSpike  float64
}

func Main(mods ...sdk.Module) {
	fs := flag.NewFlagSet("aego-editor", flag.ExitOnError)
	debug := fs.Bool("debug", false, "verbose logging and gpu debug output")
	hidden := fs.Bool("hidden", false, "create the window without showing it")
	headless := fs.Bool("headless", false, "run without a window")
	frames := fs.Int("frames", 0, "run a fixed number of frames and exit")
	cpu := fs.String("cpuprofile", "", "write a cpu profile to this file")
	profile := fs.String("profile", "", "player settings profile, default when empty")
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
		Profile:    *profile,
		Supervised: os.Getenv(session.EnvSupervised) == "1",
	}
	cfg.Recent, _ = launcher.RecentPath()
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
	defer func() {
		if rec := recover(); rec != nil {
			s.crash("panic", "editor", rec, debug.Stack(), "")
			panic(rec)
		}
	}()
	if dir := os.Getenv(EnvShots); dir != "" {
		return s.shots(dir)
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
	platform.SetAppMetadata(platform.AppMetadata{Name: "Aego Editor", Version: engine.Version, Identifier: "ru.akarpov.aego.editor"})
	if cfg.Width <= 0 {
		cfg.Width = 1600
	}
	if cfg.Height <= 0 {
		cfg.Height = 900
	}
	made, dirsErr := p.EnsureDirs()
	cat := cfg.Catalog
	if cat == nil {
		cat = editors.With(builtin.Default())
	}
	h, err := app.Build(p, app.Config{
		Engine: engine.Config{
			Title:    p.Name + " — Aego Editor",
			Width:    cfg.Width,
			Height:   cfg.Height,
			Headless: cfg.Headless,
			Hidden:   cfg.Hidden,
			Debug:    cfg.Debug,
			AppID:    p.Module + ".editor",
			Trace:    true,
		},
		Logger:         cfg.Logger,
		Debug:          cfg.Debug,
		LoopScope:      engine.ScopeFrame,
		LocateBindings: true,
		Docs:           true,
		Editor:         true,
		Catalog:        cat,
		Profile:        cfg.Profile,
	}, sdk.AdaptAll(mods)...)
	if err != nil {
		return nil, err
	}
	s := &Session{Host: h, cfg: cfg, log: h.Log.With(shell.ChannelEditor)}
	s.ring = crash.NewRing(500)
	h.Log.AddSink(s.ring)
	if st, ok := module.SectionValue[base.Settings](h.Sections, base.Section); ok {
		s.crashScene, s.spikeMs = st.CrashScene, st.SpikeMs
	}
	h.Engine.Jobs().SetPanicHandler(func(rec any, stack []byte) { s.crash("panic", "worker", rec, stack, "") })
	touchRecent(cfg, p.Root, p.Name, s.log)
	s.Scripts = &scriptHost{
		bindings: h.Bindings,
		schema:   h.Types.Schema(),
		pipeline: h.Pipeline,
		importer: app.ScriptImporter(h.Importers),
		ws:       h.Workspace,
		log:      h.Log,
		modules:  h.Modules,
		fault:    s.fault,
		panicked: s.scriptPanic,
	}
	lang := langsvc.New(h.Workspace, h.Engine.Jobs())
	if p.Native {
		lang.SetNativeRoot(p.NativeDir())
	}
	if err := s.Scripts.Reload(); err != nil {
		s.log.Error("cannot publish script types", log.F("err", err.Error()))
	}
	s.unsub = h.Loader.Subscribe(func(r asset.Reloaded) {
		if r.Kind != asset.ArtScript {
			return
		}
		if err := s.Scripts.Reload(); err != nil {
			s.log.Error("cannot publish script types", log.F("err", err.Error()))
		}
	})
	s.App = shell.New(shell.Options{
		Project:     p,
		Pipeline:    h.Pipeline,
		Loader:      h.Loader,
		Types:       h.Types,
		Renderer:    h.Renderer,
		Log:         h.Log,
		Modules:     h.Modules,
		Sections:    h.Sections,
		Problems:    h.Problems,
		Settings:    h.Settings,
		Hooks:       h.Modules.Editor,
		RenderStats: h.RenderStats,
		Supervised:  cfg.Supervised,
		Paced:       !cfg.Headless,
		Scripts:     s.Scripts,
		Code:        scripteditor.NewHost(lang),
		CreatedDirs: made,
		Help:        h.Docs,
		RunSnippet:  snippets(),
		DirsErr:     dirsErr,
		NewProject:  func(dir, template string) (string, error) { return CreateProject(dir, template, "", "") },
		Crash:       func(where string, rec any, stack []byte) { s.crash("panic", where, rec, stack, "") },
		Exporter:    newExporter(p, h.Pipeline),
	})
	s.driver = &driver{App: s.App, s: s}
	return s, nil
}

func (s *Session) attachedTo(d *document.SceneDocument) bool {
	for _, a := range s.docs {
		if a.id == d.ID && a.scene == d.Scene() {
			return true
		}
	}
	return false
}

func live(docs []*document.SceneDocument, a docAttach) bool {
	for _, d := range docs {
		if d.ID == a.id && d.Scene() == a.scene {
			return true
		}
	}
	return false
}

func (s *Session) syncDocs() {
	ctx := s.App.Context()
	if ctx == nil || s.Modules == nil {
		return
	}
	docs := ctx.Docs.Docs()
	n := 0
	for _, a := range s.docs {
		if live(docs, a) {
			s.docs[n] = a
			n++
			continue
		}
		a.detach()
	}
	s.docs = s.docs[:n]
	for _, d := range docs {
		if s.attachedTo(d) {
			continue
		}
		det, err := s.Modules.Attach(&module.AttachContext{World: s.Engine.MainWorld(), Scene: d.Scene(), Log: s.Log, Editor: true})
		if err != nil {
			s.log.Error("modules could not attach to the scene", log.F("path", d.Path), log.F("err", err.Error()))
			det = func() {}
		}
		s.docs = append(s.docs, docAttach{id: d.ID, scene: d.Scene(), detach: det})
	}
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
	return s.Engine.Step(s.driver)
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
			if !s.Engine.Step(s.driver) {
				break
			}
		}
		return s.Engine.Close()
	}
	return s.Engine.Run(s.driver)
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
	for i := len(s.docs) - 1; i >= 0; i-- {
		s.docs[i].detach()
	}
	s.docs = nil
	s.Host.Close()
}
