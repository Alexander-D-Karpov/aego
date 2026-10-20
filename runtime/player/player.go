package player

import (
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"aego/app/assembly"
	"aego/app/assembly/builtin"
	"aego/app/crash"
	"aego/asset"
	"aego/asset/loader"
	"aego/contracts/persistence"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/rng"
	"aego/core/trace"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/engine/recorder"
	"aego/engine/settings"
	inputmod "aego/modules/input"
	scriptsystems "aego/modules/scripting/systems"
	"aego/platform"
	"aego/render"
	renderhost "aego/render/host"
	"aego/runtime/bundle"
	"aego/save"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/script/bind"
	scriptruntime "aego/script/runtime"
	"aego/vfs"
	"aego/vfs/exportfs"
	"aego/vfs/osfs"
)

const (
	inputService = "input.service"
	WindowedMode = "windowed"
	FullMode     = "fullscreen"
)

type Config struct {
	Data      string
	Profile   string
	Window    string
	Scene     string
	LogFile   string
	Trace     string
	Replay    string
	Headless  bool
	Hidden    bool
	Frames    int
	FrameTime float64
	Debug     bool
	Seed      uint64
	NoProfile bool
	Logger    *log.Logger
	Bundle    *Bundle
}

type Player struct {
	Data     *bundle.Data
	Engine   *engine.Engine
	Loader   *loader.Loader
	Renderer *render.Renderer
	Types    *scene.Types
	Bindings *bind.Registry
	Modules  *module.Manifest
	Sections *module.Sections
	Settings *settings.Store
	Problems *problems.Service
	Scripts  *scriptruntime.Runtime
	Scene    *scene.Scene
	Runner   *frame.Runner
	Log      *log.Logger

	cfg        Config
	log        *log.ChannelLogger
	sceneID    ids.AssetID
	detach     func()
	unrec      func()
	ring       *crash.Ring
	profileDir string
	profileErr error
	crashDir   string
	crashScene bool
	closers    []func()
	closed     bool
}

type driver struct{}

func (driver) Init(*engine.Engine) error { return nil }
func (driver) Event(platform.Event)      {}
func (driver) FixedUpdate(float64)       {}
func (driver) Update(float64)            {}
func (driver) Render(float64)            {}
func (driver) Shutdown()                 {}

func Mount(dir string) (*bundle.Data, *vfs.VFS, error) {
	f, err := osfs.Dir{Root: dir}.Open(bundle.ManifestFile)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.AssetNotMounted, "the game data has no manifest").With(log.F("dir", dir))
	}
	raw, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.VFSIO, "cannot read the game manifest").With(log.F("dir", dir))
	}
	d, err := bundle.DecodeData(raw, bundle.ManifestFile)
	if err != nil {
		return nil, nil, err
	}
	files := map[string]int64{}
	prefix := bundle.Scheme + "://"
	d.Index.Each(func(e *asset.Entry) {
		for _, a := range e.Artifacts {
			if rel, ok := strings.CutPrefix(a.Path, prefix); ok {
				files[rel] = a.Size
			}
		}
	})
	v, err := vfs.New(vfs.Mount{Scheme: bundle.Scheme, Source: exportfs.New(dir, files), ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	return d, v, nil
}

func Open(cfg Config, mods ...module.Module) (*Player, error) {
	p := &Player{cfg: cfg, Runner: frame.New(), ring: crash.NewRing(500)}
	if err := p.build(mods); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func (p *Player) build(mods []module.Module) error {
	data, files, err := p.mount()
	if err != nil {
		return err
	}
	p.Data = data
	g := &data.Game
	if !p.cfg.NoProfile {
		p.profileDir, p.profileErr = platform.ProfileDir(g.Module, p.cfg.Profile)
		if p.profileErr == nil && hostFiles {
			p.crashDir = filepath.Join(p.profileDir, "crash")
		}
	}
	if err := p.logger(); err != nil {
		return err
	}
	p.log = p.Log.With(log.Engine)
	ecfg := engine.Config{
		Title:             g.Window.Title,
		Width:             g.Window.Width,
		Height:            g.Window.Height,
		Headless:          p.cfg.Headless,
		HeadlessFrameTime: p.cfg.FrameTime,
		Hidden:            p.cfg.Hidden,
		Debug:             p.cfg.Debug,
		AppID:             g.Module,
		Trace:             p.cfg.Trace != "",
		Seed:              p.cfg.Seed,
		ResRoot:           p.cfg.Data,
	}
	rmod := renderhost.New(render.Options{MaxSprites: 65536})
	builder := engine.NewBuilder(ecfg).WithLogger(p.Log).Use(rmod)
	if p.Types, err = assembly.NewTypes(); err != nil {
		return err
	}
	p.Bindings = bind.New()
	if err = frame.Register(builder.Registry()); err != nil {
		return err
	}
	if p.Modules, err = builtin.Runtime().Load(p.Types, p.Bindings, assembly.Options{Seed: p.cfg.Seed}, mods...); err != nil {
		return err
	}
	if p.Sections, err = p.Modules.LoadSections(g.Block, bundle.CoreBlocks...); err != nil {
		return err
	}
	if ts, ok := module.SectionValue[base.Settings](p.Sections, base.Section); ok {
		builder.Configure(ts.Apply)
		p.crashScene = ts.CrashScene
	}
	builder.Use(p.Modules.Engine())
	if err = p.Types.Schema().Freeze(); err != nil {
		return err
	}
	if err = scriptsystems.Freeze(p.Bindings, p.Types.Schema()); err != nil {
		return errs.Wrap(err, errs.EngineModule, "script bindings are invalid")
	}
	if p.Engine, err = builder.Build(); err != nil {
		return err
	}
	p.Engine.Jobs().SetPanicHandler(func(rec any, stack []byte) { p.crash("panic", "worker", rec, stack, "") })
	p.Renderer = rmod.Renderer()
	p.Loader = loader.New(p.Engine.GPU(), p.Renderer.Textures(), p.Log)
	if err = p.Loader.Mount(data.Index, files); err != nil {
		return err
	}
	p.Engine.SetService(loader.ServiceID, p.Loader)
	p.Engine.SetService(renderhost.ServiceID, p.Renderer)
	p.Problems = problems.New()
	p.Problems.Add(p.Modules.AssetProblems(p.Types.Schema(), p.Loader))
	p.Engine.SetService(problems.ServiceID, p.Problems)
	if p.Settings, err = p.Modules.NewSettings(); err != nil {
		return err
	}
	p.Settings.SetNames(p.Sections.Names)
	p.Engine.SetService(settings.ServiceID, p.Settings)
	if err = p.Modules.Init(&module.InitContext{Engine: p.Engine, Assets: p.Loader, Log: p.Log, Sections: p.Sections, Problems: p.Problems, Settings: p.Settings}); err != nil {
		return err
	}
	p.Settings.Freeze()
	p.Settings.LoadProject(g.Defaults, bundle.ManifestFile)
	p.loadProfile()
	p.Problems.Add(p.Settings.Problems())
	p.Settings.Start()
	p.applyWindow()
	for _, w := range p.Modules.Warnings {
		p.log.Warn(w)
	}
	p.log.Info("game started", log.F("game", g.Name), log.F("version", g.Version), log.F("profile", data.Profile), log.F("data", p.cfg.Data))
	return p.start()
}

func (p *Player) logger() error {
	if p.cfg.Logger != nil {
		p.Log = p.cfg.Logger
		p.Log.AddSink(p.ring)
		return nil
	}
	p.Log = log.New(log.FuncSink(func(log.Record) {}))
	release := p.Data.Release()
	if !release || p.cfg.Debug || !hostFiles {
		p.Log.AddSink(log.NewTextSink(os.Stderr))
	}
	path := p.cfg.LogFile
	if path == "" && release && p.profileDir != "" && hostFiles {
		path = filepath.Join(p.profileDir, "logs", "game.log")
	}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot create the log directory").With(log.F("path", path))
		}
		f, err := os.Create(path)
		if err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot create the log file").With(log.F("path", path))
		}
		p.Log.AddSink(log.NewTextSink(f))
		p.closers = append(p.closers, func() { f.Close() })
	}
	level := log.Info
	if p.cfg.Debug {
		level = log.Debug
	}
	p.Log.SetLevel(level)
	p.Log.AddSink(p.ring)
	return nil
}

func (p *Player) loadProfile() {
	if p.cfg.NoProfile {
		return
	}
	if p.profileErr != nil {
		p.log.Warn("player settings are not saved", log.F("err", p.profileErr.Error()))
		return
	}
	path := filepath.Join(p.profileDir, platform.ProfileFile)
	p.Engine.SetService(persistence.StoreID, save.NewStore(filepath.Join(p.profileDir, "saves")))
	data, err := platform.ReadProfile(path)
	if err != nil {
		p.log.Warn("cannot read the player settings", log.F("path", path), log.F("err", err.Error()))
	}
	if lerr := p.Settings.LoadProfile(data, path); lerr != nil && len(data) > 0 {
		_ = platform.WriteProfile(path+".bad", data)
		p.log.Warn("the player settings are damaged, a copy is kept as settings.aetext.bad", log.F("path", path))
	}
	p.Settings.SetPersist(func(b []byte) error { return platform.WriteProfile(path, b) })
}

func (p *Player) applyWindow() {
	if p.cfg.Window == "" {
		return
	}
	d, ok := p.Engine.Window().(platform.Displayer)
	if !ok {
		return
	}
	mode := platform.Windowed
	size := platform.DisplayMode{W: p.Data.Game.Window.Width, H: p.Data.Game.Window.Height}
	if p.cfg.Window == FullMode {
		mode, size = platform.Fullscreen, platform.DisplayMode{}
	}
	if err := d.SetWindowMode(mode, d.WindowDisplay(), size); err != nil {
		p.log.Warn("cannot apply the window mode from the command line", log.F("mode", p.cfg.Window), log.F("err", err.Error()))
	}
}

func (p *Player) start() error {
	p.Engine.SetView(frame.ServiceID, p.Runner)
	p.Scripts = scriptruntime.New(scriptruntime.Options{Bindings: p.Bindings, Schema: p.Types.Schema(), Log: p.Log, OnFault: p.fault, OnPanic: p.scriptPanic})
	mods, err := Scripts(p.Loader)
	if err != nil {
		return err
	}
	if err := p.Scripts.Link(mods); err != nil {
		return err
	}
	p.Engine.SetService(scriptruntime.ServiceID, p.Scripts)
	p.Engine.SetView(persistence.HostID, p)
	id, err := p.startScene()
	if err != nil {
		return err
	}
	p.sceneID = id
	s, err := p.loadScene()
	if err != nil {
		return err
	}
	p.Scene = s
	w := p.Engine.MainWorld()
	w.Recorder = recorder.New(4096)
	if p.cfg.Seed != 0 {
		w.Random = rng.New(p.cfg.Seed)
	}
	p.bindScene()
	if err := p.Scripts.Instantiate(p.Scene); err != nil {
		return err
	}
	if err := p.attach(); err != nil {
		return err
	}
	return p.replay()
}

func (p *Player) bindScene() {
	if p.unrec != nil {
		p.unrec()
	}
	p.unrec = frame.Record(p.Scene, p.Engine.MainWorld().Recorder)
	p.Runner.Bind(p.Scene, p.Renderer, p.Loader)
	p.Engine.SetService(scene.ServiceID, p.Scene)
}

func (p *Player) attach() error {
	if p.detach != nil {
		p.detach()
		p.detach = nil
	}
	d, err := p.Modules.Attach(&module.AttachContext{World: p.Engine.MainWorld(), Scene: p.Scene, Log: p.Log, Play: true})
	if err != nil {
		return err
	}
	p.detach = d
	return nil
}

func (p *Player) startScene() (ids.AssetID, error) {
	s := strings.TrimSpace(p.cfg.Scene)
	if s == "" {
		if p.Data.Game.MainScene.IsZero() {
			return ids.AssetID{}, errs.New(errs.AssetNotFound, "the game has no start scene")
		}
		return p.Data.Game.MainScene, nil
	}
	if id, err := ids.ParseAssetID(s); err == nil {
		return id, nil
	}
	want := strings.TrimSuffix(filepath.ToSlash(s), ".aescene")
	var found ids.AssetID
	p.Data.Index.Each(func(e *asset.Entry) {
		if found.IsZero() && e.Kind == asset.ArtScene && (e.Name == want || strings.TrimSuffix(e.Source, ".aescene") == want) {
			found = e.ID
		}
	})
	if found.IsZero() {
		return ids.AssetID{}, errs.New(errs.AssetNotFound, "the game data has no scene "+s)
	}
	return found, nil
}

func (p *Player) sceneName() string {
	if e, ok := p.Data.Index.Lookup(p.sceneID); ok && e.Source != "" {
		return e.Source
	}
	return p.sceneID.String()
}

func (p *Player) loadScene() (*scene.Scene, error) {
	src, ok := p.Loader.Scene(p.sceneID)
	if !ok {
		return nil, errs.New(errs.AssetNotFound, "the scene is not in the game data", log.F("asset", p.sceneID.String()))
	}
	s, report, err := sceneio.Load(src, p.sceneName(), p.Types, p.Loader, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return nil, err
	}
	for _, w := range report.Warnings {
		p.log.Warn("scene warning", log.F("path", w.Path), log.F("line", w.Pos.Line), log.F("msg", w.Msg))
	}
	for _, u := range report.Unknown {
		p.log.Warn("unknown component preserved", log.F("type", u))
	}
	return s, nil
}

func (p *Player) replay() error {
	if p.cfg.Replay == "" {
		return nil
	}
	data, err := os.ReadFile(p.cfg.Replay)
	if err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot read the input recording").With(log.F("path", p.cfg.Replay))
	}
	svc, ok := engine.ServiceOf[*inputmod.Service](p.Engine.MainWorld(), inputService)
	if !ok {
		return errs.New(errs.EngineModule, "the input module is not attached, the recording cannot be replayed")
	}
	meta, err := svc.Replay(data)
	if err != nil {
		return err
	}
	p.log.Info("replaying input", log.F("path", p.cfg.Replay), log.F("project", meta.Project), log.F("seed", meta.Seed))
	return nil
}

func (p *Player) SceneID() ids.AssetID {
	return p.sceneID
}

func (p *Player) LoadScene(id ids.AssetID) error {
	if id.IsZero() {
		return errs.New(errs.AssetNotFound, "the save names no scene")
	}
	if _, ok := p.Loader.Scene(id); !ok {
		return errs.New(errs.AssetNotFound, "the saved scene is not in this build", log.F("asset", id.String()))
	}
	prev := p.sceneID
	p.sceneID = id
	s, err := p.loadScene()
	if err != nil {
		p.sceneID = prev
		return err
	}
	p.Scene = s
	p.bindScene()
	if err := p.Scripts.Instantiate(s); err != nil {
		p.log.Error("scripts could not start on the loaded scene", log.F("err", err.Error()))
	}
	return p.attach()
}

func (p *Player) Run() error {
	defer p.Close()
	defer func() {
		if rec := recover(); rec != nil {
			p.crash("panic", "run", rec, debug.Stack(), "")
			panic(rec)
		}
	}()
	defer p.watchInterrupt()()
	d := driver{}
	if p.cfg.Frames > 0 {
		for i := 0; i < p.cfg.Frames; i++ {
			if !p.Engine.Step(d) {
				break
			}
		}
		return p.Engine.Err()
	}
	return p.Engine.Run(d)
}

func (p *Player) writeTrace() {
	f, err := os.Create(p.cfg.Trace)
	if err != nil {
		p.log.Error("cannot write the trace", log.F("path", p.cfg.Trace), log.F("err", err.Error()))
		return
	}
	t := p.Engine.Trace()
	if err := trace.WriteChrome(f, t, t.Snapshot(t.Frames())); err != nil {
		p.log.Error("cannot write the trace", log.F("path", p.cfg.Trace), log.F("err", err.Error()))
	}
	f.Close()
}

func (p *Player) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.detach != nil {
		p.detach()
		p.detach = nil
	}
	if p.unrec != nil {
		p.unrec()
		p.unrec = nil
	}
	if p.Scripts != nil {
		p.Scripts.Close()
	}
	if p.Runner != nil {
		p.Runner.Close()
	}
	if p.cfg.Trace != "" && p.Engine != nil {
		p.writeTrace()
	}
	if p.Modules != nil {
		p.Modules.Shutdown()
	}
	if p.Engine != nil {
		p.Engine.Close()
	}
	if p.Loader != nil {
		p.Loader.Close()
	}
	for i := len(p.closers) - 1; i >= 0; i-- {
		p.closers[i]()
	}
}
