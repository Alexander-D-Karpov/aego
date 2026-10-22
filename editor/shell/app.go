package shell

import (
	"path"
	"time"

	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/contracts/persistence"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/assetfiles"
	"aego/editor/core/assetops"
	edclip "aego/editor/core/clipboard"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/dataedit"
	"aego/editor/core/document"
	"aego/editor/core/favorites"
	"aego/editor/core/fieldgizmo"
	"aego/editor/core/gizmo"
	"aego/editor/core/play"
	pd "aego/editor/core/playdiff"
	"aego/editor/core/prefabs"
	"aego/editor/core/presets"
	"aego/editor/core/preview"
	"aego/editor/core/refs"
	"aego/editor/core/sceneops"
	"aego/editor/core/selection"
	"aego/editor/core/services"
	"aego/editor/core/session"
	"aego/editor/core/shape"
	"aego/editor/core/tasks"
	"aego/editor/core/templates"
	"aego/editor/core/vcs"
	"aego/editor/core/vectors"
	"aego/editor/core/workspace"
	"aego/editor/ext"
	"aego/editor/ext/inspect"
	"aego/editor/panels/assets"
	"aego/editor/panels/batchrename"
	"aego/editor/panels/console"
	"aego/editor/panels/crashes"
	"aego/editor/panels/datatable"
	edocs "aego/editor/panels/docs"
	"aego/editor/panels/events"
	exportpanel "aego/editor/panels/export"
	"aego/editor/panels/gameview"
	"aego/editor/panels/health"
	"aego/editor/panels/help"
	"aego/editor/panels/hierarchy"
	"aego/editor/panels/inspector"
	playdiffpanel "aego/editor/panels/playdiff"
	eproblems "aego/editor/panels/problems"
	"aego/editor/panels/profiler"
	"aego/editor/panels/projectsettings"
	"aego/editor/panels/references"
	"aego/editor/panels/saves"
	"aego/editor/panels/sceneview"
	"aego/editor/panels/scripteditor"
	"aego/editor/panels/search"
	"aego/editor/panels/taskcenter"
	"aego/editor/panels/vcsdiff"
	"aego/editor/panels/watch"
	"aego/editor/panels/welcome"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/engine/docs"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/engine/settings"
	"aego/platform"
	"aego/platform/input"
	"aego/project"
	"aego/render"
	"aego/render/icons"
	"aego/render/text"
	"aego/render/view"
	"aego/save"
	"aego/scene"
	"aego/ui/core"
	"aego/ui/dnd"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/ui/widgets"
)

const ChannelEditor log.Channel = "editor"

type Options struct {
	Project     *project.Project
	Pipeline    *pipeline.Pipeline
	Loader      *loader.Loader
	Types       *scene.Types
	Renderer    *render.Renderer
	Log         *log.Logger
	Panels      []workspace.Desc
	Modules     *module.Manifest
	Sections    *module.Sections
	Problems    *problems.Service
	Settings    *settings.Store
	Hooks       []module.Hook
	RenderStats func() render.Stats
	Supervised  bool
	Paced       bool
	Scripts     context.ScriptHost
	Code        *scripteditor.Host
	CreatedDirs []string
	DirsErr     error
	Help        *docs.Registry
	RunSnippet  func(code string) []string
	NewProject  func(dir, template string) (string, error)
	Crash       func(where string, rec any, stack []byte)
	Exporter    services.Exporter
	Relaunch    func(target, tab string) error
	OpenPath    string
	NativeNote  string
	RunTests    func(done func(out string, failed bool)) error
	Recent      string
	LogsDir     string
}

type App struct {
	recent         []launcher.Entry
	recentNext     float64
	recentLoaded   bool
	recentLabels   []string
	testsRunning   bool
	data           *dataedit.Service
	tablePanel     *datatable.Panel
	opt            Options
	eng            *engine.Engine
	ui             *core.Context
	ctx            *context.Editor
	ws             *workspace.Workspace
	list           *draw.List
	log            *log.ChannelLogger
	unlock         func()
	modals         []modal
	drops          []string
	dropPos        mathx.Vec2
	title          string
	textOn         bool
	captured       bool
	checkAt        float64
	recoverAt      float64
	discard        bool
	quitting       bool
	templates      *templates.Registry
	shapes         *shape.Registry
	inspectors     *inspect.Registry
	gizmos         *gizmo.Registry
	hierarchy      *hierarchy.Panel
	sceneView      *sceneview.Panel
	assetsPanel    *assets.Panel
	thumbs         *preview.Service
	presets        *presets.Service
	favs           *favorites.Store
	refs           *refs.Service
	refsPanel      *references.Panel
	missing        *references.Missing
	renameDlg      *batchrename.Dialog
	diffPanel      *vcsdiff.Panel
	healthP        *health.Panel
	exportP        *exportpanel.Panel
	native         *nativeMenu
	taskPanel      *taskcenter.Panel
	playDiff       *playdiffpanel.Dialog
	diffMode       pd.Mode
	diffAll        bool
	diffFull       pd.Result
	diffPending    bool
	diffDoc        document.ID
	diffRes        pd.Result
	tasks          *tasks.Queue
	files          *assetfiles.Files
	prefabs        *prefabs.Service
	gameView       *gameview.Panel
	profiler       *profiler.Panel
	viewport       *services.Viewport
	settings       *projectsettings.Panel
	consoleSink    *console.Sink
	play           *play.Controller
	playReturn     document.ID
	ext            *ext.Registry
	restart        bool
	sessionAt      float64
	buildSeen      time.Time
	lastInput      float64
	lastFrame      time.Time
	buildState     widgets.DotState
	consolePanel   *console.Panel
	inspectorPanel *inspector.Panel
	problemsPanel  *eproblems.Panel
	eventsPanel    *events.Panel
	watchPanel     *watch.Panel
	crashesPanel   *crashes.Panel
	savesPanel     *saves.Panel
	search         *search.Overlay
	shiftAt        float64
	shiftArmed     bool
	welcome        *welcome.Panel
	docs           *edocs.Panel
	keymap         *help.Keymap
	welcomeAt      float64
	genAtPlay      uint64
	wasPlaying     bool
	pulse          pulse
	playRect       mathx.Rect
	full           bool
	scriptLog      *log.ChannelLogger
	problemRev     uint64
	clip           *clipboard
	picker         scenePicker
}

func New(opt Options) *App {
	return &App{opt: opt}
}

func (a *App) Context() *context.Editor {
	return a.ctx
}

func (a *App) Workspace() *workspace.Workspace {
	return a.ws
}

func (a *App) Data() *dataedit.Service {
	return a.data
}

func (a *App) Play() *play.Controller {
	return a.play
}

func (a *App) Extensions() *ext.Registry {
	return a.ext
}

func (a *App) Templates() *templates.Registry {
	return a.templates
}

func (a *App) Profiler() *profiler.Panel {
	return a.profiler
}

func (a *App) Inspector() *inspector.Panel {
	return a.inspectorPanel
}

func (a *App) Prefabs() *prefabs.Service {
	return a.prefabs
}

func (a *App) Viewport() *services.Viewport {
	return a.viewport
}

// UIList is for golden tests only; it returns the last built UI frame.
func (a *App) UIList() *draw.List {
	return a.list
}

func (a *App) Init(e *engine.Engine) error {
	a.eng = e
	lg := a.opt.Log
	if lg == nil {
		lg = e.Log()
	}
	a.log = lg.With(ChannelEditor)
	a.consoleSink = console.NewSink(4000)
	lg.AddSink(a.consoleSink)
	a.reportDirs()
	r := a.opt.Renderer
	ui := core.New(r.UIFont())
	ui.Theme = theme.Editor()
	if sh, err := draw.NewShapes(e.GPU(), r.Textures()); err == nil {
		ui.SetShapes(sh)
	} else {
		a.log.Warn("ui shapes are unavailable, corners stay square", log.F("err", err.Error()))
	}
	if set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetFonts(set)
	} else {
		a.log.Warn("ui fonts are missing, using the debug font", log.F("err", err.Error()))
	}
	if set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui-mono", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetMonoFonts(set)
	} else {
		a.log.Warn("code fonts are missing, the script editor uses the ui font", log.F("err", err.Error()))
	}
	a.clip = &clipboard{win: e.Window(), now: e.Platform().Now}
	ui.Clipboard = a.clip
	a.ui = ui
	if set, err := icons.Load(e.VFS(), r.Textures(), "engine://icons"); err == nil {
		set.OnMissing = func(id string) {
			a.log.Warn("icon is missing from the atlas", log.F("id", id))
		}
		ui.Icons = set
	} else {
		a.log.Warn("icon atlas is missing, run make genicons", log.F("err", err.Error()))
	}

	var res asset.Resolver
	if a.opt.Loader != nil {
		res = a.opt.Loader
	}
	p := a.opt.Project
	a.shapes = shape.NewRegistry()
	a.ctx = &context.Editor{
		Engine:      e,
		Renderer:    r,
		Project:     p,
		Pipeline:    a.opt.Pipeline,
		Loader:      a.opt.Loader,
		Types:       a.opt.Types,
		Docs:        document.NewManager(document.Options{Root: p.Root, Types: a.opt.Types, Resolver: res, Log: lg, OnChange: module.RunChanged}),
		Selection:   selection.New(),
		Commands:    command.New(),
		UI:          ui,
		DnD:         &dnd.State{},
		Clipboard:   a.clip,
		Log:         a.log,
		Settings:    context.DefaultSettings(),
		RenderStats: a.opt.RenderStats,
		Scripts:     a.opt.Scripts,
		Collapsed:   map[string]bool{},
		Shapes:      a.shapes,
		Sections:    a.opt.Sections,
	}
	a.ctx.Problems = a.opt.Problems
	if a.ctx.Problems == nil {
		a.ctx.Problems = problems.New()
	}
	a.ctx.Problems.Add(a.ctx.SceneChecks())
	a.ctx.Commands.OnError = func(id string, err error) {
		a.ctx.Notify(context.LevelError, err.Error())
	}
	a.ctx.DnD.Trace = func(event string, p dnd.Payload) {
		a.log.Debug("dnd "+event, log.F("kind", p.Kind), log.F("label", p.Label))
	}

	a.templates = templates.New()
	if a.opt.Modules != nil {
		if err := a.templates.Load(a.opt.Modules.Templates); err != nil {
			a.ctx.Notify(context.LevelError, err.Error())
		}
	}
	a.inspectors = inspect.NewRegistry()
	if err := a.inspectors.Load(a.opt.Types.Schema()); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
	}
	a.inspectors.SetEditor(a.ctx)
	if a.opt.Modules != nil {
		ids := make([]string, len(a.opt.Modules.Modules))
		for i, m := range a.opt.Modules.Modules {
			ids[i] = m.ID
		}
		a.inspectors.SetModules(ids)
	}
	a.gizmos = gizmo.NewRegistry()
	if err := fieldgizmo.Register(a.ctx, a.gizmos); err != nil {
		return err
	}
	a.play = play.New(a.ctx)
	a.play.SetBeforeStop(a.beforeStop)
	a.ctx.Play = a.play
	a.play.Subscribe(func(st play.State) {
		if st == play.Stopped {
			for _, d := range a.ctx.Docs.Docs() {
				a.ctx.PruneSelectionOf(d.ID)
			}
			a.afterPlay()
			if a.data != nil {
				a.data.PlayStopped()
			}
		}
	})
	if a.opt.Modules != nil {
		for _, m := range a.opt.Modules.Modules {
			if m.NonDeterministic != "" {
				a.play.SetBlocked("Rewind is off: module " + m.ID + " " + m.NonDeterministic)
				break
			}
		}
	}
	a.files = assetfiles.New(a.ctx)
	a.prefabs = prefabs.New(a.ctx)
	a.ctx.BeforeSwap = a.prefabs.External
	a.data = dataedit.New(a.ctx)
	a.data.OnLive = func() { a.play.Invalidate("a data asset was tuned during Play") }
	a.ctx.Selection.Subscribe(a.data.Clear)
	a.tablePanel = datatable.New(datatable.Deps{Data: a.data, Notices: a.ctx})
	a.templates.SetPrefabs(a.prefabs)
	a.tasks = tasks.New(tasks.Deps{
		Submit: func(fn func()) { e.Jobs().Submit(fn) },
		OnMain: func(fn func()) { e.Jobs().OnMain(fn) },
		Budget: 4 * time.Millisecond,
	})
	a.thumbs = preview.New(preview.Deps{
		Engine:     e,
		Pipeline:   a.opt.Pipeline,
		Renderer:   r,
		GPU:        e.GPU(),
		Types:      a.opt.Types,
		Resolver:   res,
		Shapes:     a.shapes,
		Tasks:      a.tasks,
		CacheRoot:  p.CacheDir(),
		Background: ui.Theme.Background,
		Cover:      func() ids.AssetID { return a.opt.Project.MainScene },
		Reference: func() mathx.Vec2 {
			w := a.opt.Project.Window
			return mathx.Vec2{X: float32(w.Width), Y: float32(w.Height)}
		},
	})
	a.ctx.Thumbs = a.thumbs
	sops := sceneops.New(a.ctx, a.files.History())
	a.refs = refs.New(a.ctx)
	a.refs.Prefabs = a.prefabs
	rd := references.Deps{
		Refs: a.refs, Pipeline: a.opt.Pipeline, Docs: a.ctx, Selection: a.ctx, Nav: a.ctx, Notices: a.ctx, Tasks: a.tasks,
		Delete:        func(list []ids.AssetID) { a.assetsPanel.DeleteNow(list) },
		Show:          func(id ids.AssetID) { a.showReferences(id, nil) },
		ShowHierarchy: func() { a.ws.Show(workspace.Hierarchy) },
	}
	a.refsPanel = references.New(rd)
	a.missing = references.NewMissing(rd)
	a.renameDlg = batchrename.New(batchrename.Deps{
		Pipeline: a.opt.Pipeline,
		Docs:     a.ctx,
		Exists:   func(rel string) bool { return assetops.Exists(a.ctx, rel) },
		Move:     a.moveAssets,
		Notices:  a.ctx,
	})
	prov, _ := vcs.Detect(p.Root)
	a.diffPanel = vcsdiff.New(vcsdiff.Deps{VCS: prov, Pipeline: a.opt.Pipeline, Docs: a.ctx, Selection: a.ctx, Notices: a.ctx, Tasks: a.tasks,
		ShowHierarchy: func() { a.ws.Show(workspace.Hierarchy) }})
	a.healthP = health.New(a.healthDeps())
	if err := a.initExport(); err != nil {
		return err
	}
	a.taskPanel = taskcenter.New(a.tasks)
	var compare func(string)
	if prov != nil {
		compare = a.compare
	}
	a.favs = favorites.Load(p.Root)
	if err := a.favs.Err(); err != nil {
		a.log.Warn("favorites are unreadable", log.F("err", err.Error()))
	}
	a.assetsPanel = assets.New(assets.Deps{
		Favorites:      a.favs,
		Refs:           a.refs,
		ShowReferences: a.showReferences,
		BatchRename:    a.batchRenameAssets,
		Compare:        compare,
		Schema:         a.opt.Types.Schema(),
		Inspect:        func(id ids.AssetID) { a.data.Inspect(id) },
		Viewable:       a.viewable,
		OpenTable:      a.openTable,
		DataType:       a.data.TypeOf,
		Files:          a.files,
		Pipeline:       a.opt.Pipeline,
		Thumbs:         a.thumbs,
		Docs:           a.ctx,
		Selection:      a.ctx,
		Scenes:         sops,
		Commands:       a.ctx.Commands,
		Notices:        a.ctx,
		Nav:            a.ctx,
		Project:        p,
		Prefabs:        a.prefabs,
	})
	a.assetsPanel.Instantiate = a.instantiatePrefab
	a.hierarchy = hierarchy.New(hierarchy.Deps{
		Templates: a.templates,
		Docs:      a.ctx,
		Selection: a.ctx,
		Commands:  a.ctx.Commands,
		Scenes:    sops,
		Notices:   a.ctx,
		Tools:     &a.ctx.Settings,
		Prefabs:   a.prefabs,
	})
	a.viewport = &services.Viewport{
		UI:       ui,
		GPU:      e.GPU(),
		Renderer: r,
		Shapes:   a.shapes,
		Overlay:  &a.ctx.Overlay,
		OnRender: a.ctx.OnRender,
		RenderWorld: func(w *engine.World) {
			e.RunPhase(w, engine.PhaseRenderPrepare, engine.ScopeEditor, 0, 0)
			e.RunPhase(w, engine.PhaseRender, engine.ScopeEditor, 0, 0)
		},
	}
	a.sceneView = sceneview.New(a.gizmos, sceneview.Deps{
		Capture:   a.captureTexture,
		Viewport:  a.viewport,
		Resolver:  res,
		Docs:      a.ctx,
		Selection: a.ctx,
		Tools:     &a.ctx.Settings,
		Commands:  a.ctx.Commands,
		Notices:   a.ctx,
		Scenes:    sops,
		Files:     a.files,
		Pipeline:  a.opt.Pipeline,
		Project:   p,
		Templates: a.templates,
	})
	a.ctx.NewScene = a.createScene
	a.ctx.NewPrefab = a.createPrefab
	a.gameView = gameview.New(a.play, a.viewport)
	a.profiler = profiler.New(profiler.Deps{
		Tracer:      e.Trace(),
		Root:        p.Root,
		Notices:     a.ctx,
		Nav:         a.ctx,
		PhaseAllocs: e.PhaseAllocs,
		SetProbe:    e.SetPhaseAllocs,
		Sections:    func() []context.ProfilerSection { return a.ctx.Profiler },
		Throttled:   func() bool { return a.opt.Paced && a.idle() },
	})

	a.ws = workspace.New(p.Root, a.log)
	a.ws.Notices, a.ws.Tracer = a.ctx, e.Trace()
	a.consolePanel = console.New(a.consoleSink)
	a.problemsPanel = eproblems.New(a.ctx, a.ctx)
	a.problemsPanel.Scene = func(path string) *scene.Scene {
		if d := a.ctx.Docs.Find(path); d != nil {
			return d.Scene()
		}
		return nil
	}
	playWorld := func() *engine.World { return a.play.World() }
	a.eventsPanel = events.New(events.Deps{World: playWorld, Selection: a.ctx, Notices: a.ctx, Root: p.Root})
	a.watchPanel = watch.New(watch.Deps{World: playWorld, Scripts: func() context.ScriptHost { return a.ctx.Scripts }, Viewport: a.viewport})
	a.crashesPanel = crashes.New(p.Root, a.ctx)
	a.savesPanel = saves.New(saves.Deps{
		Store: func() *save.Store {
			st, _ := engine.Service[*save.Store](e, persistence.StoreID)
			return st
		},
		Service: func() persistence.Service {
			w := a.play.World()
			if w == nil {
				return nil
			}
			s, _ := engine.ServiceOf[persistence.Service](w, persistence.ServiceID)
			return s
		},
		Label: func(id ids.AssetID) string {
			if a.opt.Pipeline != nil {
				if rel, ok := a.opt.Pipeline.PathOf(id); ok {
					return rel
				}
			}
			return id.Short()
		},
		Notices: a.ctx,
	})
	a.ctx.Crash = a.opt.Crash
	a.ws.OnCrash = func(id string, rec any, stack []byte) {
		if a.ctx.Crash != nil {
			a.ctx.Crash("panel "+id, rec, stack)
		}
	}
	a.ctx.ShowEvents = func(frame uint64) {
		a.ws.Show(events.ID)
		a.eventsPanel.Focus(frame)
	}
	a.play.SetBreak(func() (string, bool) {
		if why, ok := a.eventsPanel.Check(); ok {
			return why, true
		}
		return a.watchPanel.Check()
	})
	a.presets = presets.New(a.ctx)
	a.inspectorPanel = inspector.New(a.inspectors, inspector.Deps{
		Presets:   a.presets,
		Data:      a.data,
		Schema:    a.opt.Types.Schema(),
		Docs:      a.ctx,
		Selection: a.ctx,
		Commands:  a.ctx.Commands,
		Notices:   a.ctx,
		Nav:       a.ctx,
		Problems:  a.ctx,
		Sections:  a.opt.Sections,
		Pipeline:  a.opt.Pipeline,
		Thumbs:    a.thumbs,
		AssetView: a.assetView,
		Audition:  a.audition,
		Play:      a.play,
		Code:      func() context.ScriptNav { return a.ctx.Code },
		Clip:      edclip.EnvOf(a.ctx),
		Assets:    sops,
		Scenes:    sops,
		Collapsed: a.ctx.Collapsed,
		Watch: func(expr string) {
			a.watchPanel.Add(expr)
			a.ws.Show(watch.ID)
		},
		Prefabs: a.prefabs,
	})
	a.docs = edocs.New(a.opt.Help, a.ctx, a.ctx)
	a.docs.Run = a.opt.RunSnippet
	a.ctx.ShowDocs = func(ref, query string) {
		a.ws.Show(edocs.ID)
		a.docs.Show(ref, query)
	}
	a.keymap = help.NewKeymap(a.ctx.Commands)
	a.settings = projectsettings.New(projectsettings.Deps{
		Project:  p,
		Sections: a.opt.Sections,
		Settings: a.opt.Settings,
		OnApply:  a.liveSettings,
		Pages:    func() []context.SettingsPage { return a.ctx.Pages },
		Pipeline: a.opt.Pipeline,
		Notices:  a.ctx,
	})
	a.welcome = welcome.New(p.Root, a.welcomeChecks())
	a.welcome.Heading, _ = a.helpEntry("welcome")
	_, a.welcome.Outro = a.helpEntry("welcome/done")
	a.welcome.Reference = func() { a.ctx.ShowDocs(edocs.Language, "") }
	if a.opt.NewProject != nil {
		a.welcome.NewProject = a.askNewProject
		if a.opt.Relaunch != nil {
			a.welcome.NewProject = func() { a.switchTo("", tabNew) }
		}
		a.welcome.Empty = a.emptyProject
	}
	if err := a.registerPanels(); err != nil {
		return err
	}
	a.scriptLog = lg.With(log.Channel("script"))
	a.ctx.ShowProblems = func(filter string) {
		a.ws.Show(workspace.Problems)
		a.problemsPanel.Focus(filter)
	}
	a.ctx.RevealPath = func(rel string) {
		a.ws.Show(workspace.Assets)
		a.assetsPanel.Reveal(rel)
	}
	a.search = search.New(search.Deps{
		Favorites: a.favs,
		Pipeline:  a.opt.Pipeline,
		Symbols: func() []context.SearchItem {
			if a.ctx.Code == nil {
				return nil
			}
			return a.ctx.Code.Symbols()
		},
		Docs:      a.ctx,
		Selection: a.ctx,
		Commands:  a.ctx.Commands,
		Notices:   a.ctx,
		Nav:       a.ctx,
		Settings:  a.settingItems,
	})
	a.search.ShowHierarchy = func() { a.ws.Show(workspace.Hierarchy) }
	a.attachCode()
	if err := a.registerCommands(); err != nil {
		return err
	}
	if err := a.registerMeta(); err != nil {
		return err
	}
	if err := a.registerTime(); err != nil {
		return err
	}
	if err := a.registerSearch(); err != nil {
		return err
	}
	if err := a.registerHelp(); err != nil {
		return err
	}
	if err := a.registerProject(); err != nil {
		return err
	}
	if err := a.registerWorkspaces(); err != nil {
		return err
	}
	if err := a.registerEditExtras(); err != nil {
		return err
	}
	if err := a.registerCapture(); err != nil {
		return err
	}
	if err := a.registerReferences(); err != nil {
		return err
	}
	if err := a.registerAssetTools(); err != nil {
		return err
	}
	a.playDiff = playdiffpanel.New(playdiffpanel.Deps{Docs: a.ctx, Notices: a.ctx,
		ShowAll: func() bool { return a.diffAll }, SetShowAll: func(on bool) { a.diffAll = on }})
	if err := a.registerPlayDiff(); err != nil {
		return err
	}
	a.ext = ext.New(ext.Deps{
		Editor:     a.ctx,
		Workspace:  a.ws,
		Inspectors: a.inspectors,
		Gizmos:     a.gizmos,
		Templates:  a.templates,
		Shapes:     a.shapes,
		Commands:   a.ctx.Commands,
		Schema:     a.opt.Types.Schema(),
	})
	for _, h := range a.opt.Hooks {
		if err := a.ext.Run(h.Owner, h.Fn); err != nil {
			a.ctx.Notify(context.LevelError, "extension "+h.Owner+" failed to register: "+err.Error())
		}
	}
	if err := vectors.Register(a.ctx, a.shapes); err != nil {
		return err
	}
	if err := gameview.Register(a.ctx.Commands, &a.ctx.Overlay); err != nil {
		return err
	}
	a.ctx.ShowSettings = func(page string) {
		a.settings.Show(page)
		a.ws.Show(projectsettings.ID)
	}
	if err := a.ctx.Commands.Register(command.Desc{
		ID: projectsettings.CommandOpen, Name: "Project Settings…", Category: "File", Order: 800,
		Run: func() error {
			a.ctx.Open(context.SettingsAt(""))
			return nil
		},
	}); err != nil {
		return err
	}
	for _, d := range []command.Desc{
		{ID: projectsettings.CommandUndo, Name: "Undo Settings Change", Scope: projectsettings.Scope,
			Shortcut: command.Key(input.KeyZ, command.ModPrimary), Enabled: a.settings.CanUndo,
			Run: func() error { a.settings.Undo(); return nil }},
		{ID: projectsettings.CommandRedo, Name: "Redo Settings Change", Scope: projectsettings.Scope,
			Shortcut: command.Key(input.KeyZ, command.ModPrimary|command.ModShift), Alt: []command.Shortcut{command.Key(input.KeyY, command.ModPrimary)},
			Enabled: a.settings.CanRedo, Run: func() error { a.settings.Redo(); return nil }},
	} {
		if err := a.ctx.Commands.Register(d); err != nil {
			return err
		}
	}
	if err := a.registerWindowCommands(); err != nil {
		return err
	}
	a.initNativeMenu()
	a.describeCommands()
	a.iconCommands()
	a.ws.LoadLayout()

	unlock, other, err := document.AcquireLock(p.Root)
	a.unlock = unlock
	if err != nil {
		a.log.Warn("cannot write the editor lock", log.F("err", err.Error()))
	}
	if other != nil {
		a.push(modal{kind: modalLock, lock: *other})
	}
	a.restoreState()
	a.buildState = widgets.DotOK
	if st, ok := session.ReadStatus(p.Root); ok {
		a.buildSeen = st.Time
	}
	if a.ctx.Docs.Active() == nil {
		a.openMainScene()
	}
	for _, rec := range a.ctx.Docs.FindRecovery() {
		a.push(modal{kind: modalRecovery, rec: rec})
	}
	if a.opt.OpenPath != "" {
		if _, err := a.ctx.Docs.Open(a.opt.OpenPath); err != nil {
			a.ctx.Notify(context.LevelError, err.Error())
		}
	}
	if a.opt.NativeNote != "" {
		a.ctx.Notify(context.LevelWarn, a.opt.NativeNote)
	}
	a.firstRun()
	now := e.Platform().Now()
	a.checkAt, a.recoverAt = now+1, now+30
	return nil
}

func (a *App) openMainScene() {
	p := a.opt.Project
	if p.MainScene.IsZero() || a.opt.Pipeline == nil {
		return
	}
	rel, ok := a.opt.Pipeline.PathOf(p.MainScene)
	if !ok {
		return
	}
	if _, err := a.ctx.Docs.Open(rel); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
	}
}

func (a *App) Event(ev platform.Event) {
	a.lastInput = a.eng.Platform().Now()
	switch ev.Kind {
	case platform.EvKeyDown:
		a.shiftTap(ev)
		a.paletteKey(ev)
	case platform.EvTextEditing:
		a.ui.SetComposition(ev.Str, int(ev.EditStart))
	case platform.EvTextInput:
		a.ui.SetComposition("", 0)
	case platform.EvWindowFocusGained:
		a.checkAt = 0
		a.clip.invalidate()
	case platform.EvWindowFocusLost:
		a.gameView.Release()
		if a.opt.Code != nil {
			a.opt.Code.FlushAutosave()
		}
	case platform.EvDropFile:
		a.drops = append(a.drops, ev.Str)
		a.dropPos = ev.Mouse.Pos
	case platform.EvDropComplete:
		if a.dropProject() {
			a.drops = nil
			break
		}
		if len(a.drops) > 0 {
			label := a.drops[0]
			if len(a.drops) > 1 {
				label = "files"
			}
			a.ctx.DnD.External(dnd.Payload{Kind: context.KindFiles, Label: label, Data: a.drops}, a.dropPos)
			a.drops = nil
		}
	case platform.EvQuit:
		if !a.quitting && a.ctx.Docs.Dirty() {
			a.eng.CancelQuit()
			a.push(modal{kind: modalQuit})
		}
	}
}

func (a *App) FixedUpdate(float64) {}

func (a *App) Update(dt float64) {
	e := a.eng
	now := e.Platform().Now()
	ui := a.ui
	ui.Time = now
	var rs render.Stats
	if a.opt.RenderStats != nil {
		rs = a.opt.RenderStats()
	}
	var sc context.ScriptStats
	hasSc := false
	if a.ctx.Scripts != nil {
		sc, hasSc = a.ctx.Scripts.Stats()
	}
	a.profiler.Record(now, e.Stats(), rs, a.play.Stats(), sc, hasSc, a.play.State() != play.Stopped)
	a.play.Tick(dt)
	a.ext.RunTicks()
	a.tickTasks()
	a.syncNativeMenu(now)
	a.showPlayDiff()
	if a.full && !a.play.Captured() {
		a.exitFull()
	}
	in := e.Input()
	switch {
	case ui.ModalActive() || a.playDiff.Active() || a.renameDlg.Active() || a.ws.Dragging() || a.ctx.DnD.Active() || a.play.Captured():
	case ui.Focus() == 0:
		a.ctx.Commands.Dispatch(in, a.ws.Scope(a.ws.Hovered()))
	default:
		a.globalKeys(in)
	}
	m := e.Metrics()
	size := mathx.Vec2{X: float32(m.LogicalW), Y: float32(m.LogicalH)}
	ui.Begin(in, size, m.Density())
	a.ctx.FocusedPanel = a.ws.Focused()
	th := &ui.Theme
	full := mathx.Rect{W: size.X, H: size.Y}
	ui.Rect(full, th.Background)
	frame := services.Frame{UI: ui, DnD: a.ctx.DnD, Now: now}
	if a.full {
		frame.Focused = true
		ui.Region(full)
		a.gameView.Draw(&frame, full)
		ui.PopCursor()
	} else {
		bh := th.BarHeight
		menuH := bh
		if a.native != nil {
			menuH = 0
		}
		tool := float32(0)
		if a.ws.Toolbar() {
			tool = bh
		}
		menuR := mathx.Rect{W: size.X, H: menuH}
		toolR := mathx.Rect{Y: menuH, W: size.X, H: tool}
		bottom := mathx.Rect{Y: size.Y - th.MenuHeight, W: size.X, H: th.MenuHeight}
		body := mathx.Rect{Y: menuH + tool, W: size.X, H: max(0, size.Y-menuH-tool-bottom.H)}
		a.ws.Draw(frame, body)
		if a.play.State() != play.Stopped {
			l := ui.List()
			l.PushLayer(draw.LayerTop)
			l.RectOutline(body, th.PlayTint, 2)
			l.PopLayer()
		}
		a.drawScriptsBanner(body)
		a.drawStatus(bottom)
		if tool > 0 {
			a.drawToolbar(toolR)
		}
		if menuH > 0 {
			a.drawMenuBar(menuR)
		}
		a.drawPulse()
	}
	for _, q := range a.ctx.TakeQuestions() {
		a.modals = append(a.modals, modal{kind: modalAsk, ask: q, vals: append([]string(nil), q.Values...), focus: true})
	}
	a.drawSearch()
	a.playDiff.Draw(ui)
	a.renameDlg.Draw(ui)
	a.drawScenePicker()
	a.drawModals()
	a.ctx.DnD.Frame(ui)
	a.ext.Flush()
	a.list = ui.End()
	a.applyPlatform()
	a.periodic(now)
}

func (a *App) Render(float64) {
	defer a.pace()
	a.ctx.RunRenderHooks()
	if a.list == nil || a.list.Empty() {
		return
	}
	m := a.eng.Metrics()
	v := view.ScreenView(m.PixelW, m.PixelH, m.Density())
	v.Clear = a.ui.Theme.Background
	v.DoClear = true
	v.Label = "editor.ui"
	r := a.opt.Renderer
	white := r.White()
	if f := a.ui.Font(); f != nil && !f.White.IsZero() {
		white = f.White
	}
	f := r.View(v)
	a.list.Flush(f, a.ui.Font(), white, 0)
	f.End()
}

func (a *App) tickTasks() {
	a.thumbs.Begin()
	a.tasks.Tick()
	if a.tasks.WantsRender() {
		a.ctx.OnRender(a.tasks.Render)
	}
}

func (a *App) Previews() *preview.Service {
	return a.thumbs
}

func (a *App) Tasks() *tasks.Queue {
	return a.tasks
}

func (a *App) idle() bool {
	now := a.eng.Platform().Now()
	return a.play.State() == play.Stopped &&
		now-a.lastInput > 1 &&
		!a.ctx.DnD.Active() &&
		!a.ws.Dragging() &&
		!a.tasks.Busy() &&
		a.ui.Captured() == 0 &&
		len(a.modals) == 0
}

func (a *App) pace() {
	if !a.opt.Paced {
		return
	}
	target := time.Second / 60
	if a.idle() {
		target = time.Second / 15
	}
	if el := time.Since(a.lastFrame); el < target {
		time.Sleep(target - el)
	}
	a.lastFrame = time.Now()
}

func (a *App) Shutdown() {
	a.play.Stop()
	a.saveState()
	if err := a.ws.SaveLayout(); err != nil {
		a.log.Warn("cannot save the layout", log.F("err", err.Error()))
	}
	if a.discard {
		a.ctx.Docs.ClearAllRecovery()
	} else if _, err := a.ctx.Docs.WriteRecovery(); err != nil {
		a.log.Warn("cannot write recovery files", log.F("err", err.Error()))
	}
	a.sceneView.Close()
	a.gameView.Close()
	a.tasks.CancelAll()
	a.thumbs.Close()
	if a.unlock != nil {
		a.unlock()
	}
}

func (a *App) requestQuit() {
	if a.ctx.Docs.Dirty() {
		a.push(modal{kind: modalQuit})
		return
	}
	a.quitting = true
	a.eng.Quit()
}

func (a *App) scriptErrors() int {
	if a.ctx.Scripts == nil {
		return 0
	}
	return a.ctx.Scripts.Errors()
}

func (a *App) reportScripts() {
	if a.ctx.Scripts == nil {
		return
	}
	ps, rev := a.ctx.Scripts.Problems()
	if rev == a.problemRev {
		return
	}
	a.problemRev = rev
	errors := 0
	for _, p := range ps {
		fields := []log.Field{log.F("path", p.Path), log.F("line", p.Line), log.F("col", p.Col), log.F("code", p.Code), log.F("package", p.Package)}
		if p.Error {
			errors++
			a.scriptLog.Error(p.Message, fields...)
		} else {
			a.scriptLog.Warn(p.Message, fields...)
		}
	}
	if errors == 0 {
		a.scriptLog.Info("scripts compiled")
		return
	}
	if !a.welcome.Done(hintErrors) {
		a.welcome.Mark(hintErrors)
		a.ctx.Notify(context.LevelInfo, "Tip: click a problem in the Problems panel to jump to its line, F2 walks through errors")
	}
}

func (a *App) periodic(now float64) {
	a.pollSession(now)
	if a.restart {
		return
	}
	a.reportScripts()
	if now >= a.welcomeAt {
		a.welcomeAt = now + 0.5
		a.welcome.Tick()
	}
	if now >= a.checkAt {
		a.checkAt = now + 1
		reloaded, conflicts := a.ctx.Docs.CheckExternal()
		for _, id := range reloaded {
			a.ctx.PruneSelectionOf(id)
			if d := a.ctx.Docs.Get(id); d != nil {
				a.ctx.Notify(context.LevelInfo, "Reloaded "+d.Path)
			}
		}
		for _, c := range conflicts {
			a.push(modal{kind: modalConflict, doc: c.Doc, path: c.Path, removed: c.Removed})
		}
	}
	if now >= a.recoverAt {
		a.recoverAt = now + 30
		if _, err := a.ctx.Docs.WriteRecovery(); err != nil {
			a.log.Warn("cannot write recovery files", log.F("err", err.Error()))
		}
	}
	a.ws.Tick(now)
	if a.opt.Code != nil {
		a.opt.Code.Tick(now)
	}
	a.updateTitle()
}

func (a *App) Restarting() bool {
	return a.restart
}

func (a *App) Handoff() error {
	a.play.Stop()
	if err := session.Write(a.opt.Project.Root, a.handoff()); err != nil {
		return err
	}
	a.restart = true
	a.quitting = true
	a.eng.Quit()
	return nil
}

func (a *App) pollSession(now float64) {
	if !a.opt.Supervised || a.restart || now < a.sessionAt {
		return
	}
	a.sessionAt = now + 0.25
	root := a.opt.Project.Root
	if st, ok := session.ReadStatus(root); ok && st.Time.After(a.buildSeen) {
		a.buildSeen = st.Time
		switch st.State {
		case session.StateBuilding:
			a.buildState = widgets.DotBusy
		case session.StateOK:
			a.buildState = widgets.DotOK
			a.ctx.Notify(context.LevelInfo, "Native code rebuilt, reloading the editor")
		default:
			a.buildState = widgets.DotError
			a.log.Error("native build failed", log.F("output", st.Log))
			a.ctx.Notify(context.LevelError, "Native build failed, the editor keeps the previous build (see Console)")
		}
	}
	if session.RestartRequested(root) {
		if err := a.Handoff(); err != nil {
			session.ClearRestart(root)
			a.ctx.Notify(context.LevelError, "cannot hand off the session: "+err.Error())
		}
	}
}

func (a *App) updateTitle() {
	t := a.opt.Project.Name + " — Aego Editor"
	if d := a.ctx.Docs.Edit(); d != nil {
		name := path.Base(d.Path)
		if d.Dirty() {
			name += "*"
		}
		switch a.play.State() {
		case play.Playing:
			name += " [Play]"
		case play.Paused:
			name += " [Paused]"
		}
		t = name + " — " + t
	}
	if a.native != nil && a.opt.Supervised {
		switch a.buildState {
		case widgets.DotBusy:
			t += " — native building"
		case widgets.DotError:
			t += " — Native API is stale"
		}
	}
	if a.opt.NativeNote != "" {
		t += " — native code not built"
	}
	if t != a.title {
		a.title = t
		a.eng.Window().SetTitle(t)
	}
}

var _ services.Prefabs = (*prefabs.Service)(nil)

func (a *App) settingItems() []search.Item {
	refs := a.settings.Index()
	out := make([]search.Item, len(refs))
	for i, r := range refs {
		out[i] = search.Item{Label: r.Label, Detail: r.Detail, Run: func() {
			a.settings.FocusRef(r)
			a.ws.Show(projectsettings.ID)
		}}
	}
	return out
}

func (a *App) liveSettings() string {
	if a.play.State() == play.Stopped || a.opt.Modules == nil {
		return ""
	}
	a.play.Invalidate("settings changed during Play")
	ctx := &module.AttachContext{World: a.play.World(), Scene: a.play.Scene(), Log: a.opt.Log, Editor: true, Play: true}
	if err := a.opt.Modules.Reconfigure(ctx); err != nil {
		a.log.Warn("settings were saved but not applied to the running game", log.F("err", err.Error()))
		return "some settings apply on the next Play"
	}
	return "applied to the running game"
}

func (a *App) viewable(k asset.ArtifactKind) bool {
	_, ok := a.ctx.AssetViews[string(k)]
	return ok
}

func (a *App) audition(id ids.AssetID, toggle bool) (bool, bool) {
	if a.opt.Pipeline == nil || len(a.ctx.AssetPlayers) == 0 {
		return false, false
	}
	e, ok := a.opt.Pipeline.Entry(id)
	if !ok {
		return false, false
	}
	key := string(e.Kind)
	if e.Kind == asset.ArtData && a.data != nil {
		if key, ok = a.data.TypeOf(id); !ok {
			return false, false
		}
	}
	pl, ok := a.ctx.AssetPlayers[key]
	if !ok {
		return false, false
	}
	playing := pl.Playing(a.ctx) == id
	if toggle {
		if playing {
			pl.Stop(a.ctx)
		} else if err := pl.Play(a.ctx, id); err != nil {
			a.ctx.Notify(context.LevelError, err.Error())
		}
		playing = pl.Playing(a.ctx) == id
	}
	return playing, true
}

func (a *App) assetView(id ids.AssetID) bool {
	if a.opt.Pipeline == nil {
		return false
	}
	e, ok := a.opt.Pipeline.Entry(id)
	if !ok {
		return false
	}
	fn, ok := a.ctx.AssetViews[string(e.Kind)]
	if !ok {
		return false
	}
	fn(a.ctx, id)
	return true
}

func (a *App) openTable(id ids.AssetID) {
	if err := a.tablePanel.Open(id); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
		return
	}
	a.ws.Show(datatable.ID)
}
