package app

import (
	"path"
	"time"

	"aego/asset"
	"aego/asset/loader"
	"aego/asset/pipeline"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/assets"
	"aego/editor/command"
	"aego/editor/console"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/extension"
	"aego/editor/gameview"
	"aego/editor/gizmo"
	"aego/editor/hierarchy"
	"aego/editor/inspector"
	"aego/editor/play"
	"aego/editor/profiler"
	"aego/editor/sceneview"
	"aego/editor/selection"
	"aego/editor/session"
	"aego/editor/templates"
	"aego/editor/workspace"
	"aego/engine"
	"aego/platform"
	"aego/project"
	"aego/render"
	"aego/render/icons"
	"aego/render/text"
	"aego/render/view"
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
	Panels      []workspace.Panel
	Extensions  []extension.Module
	RenderStats func() render.Stats
	Supervised  bool
	Paced       bool
	Scripts     context.ScriptHost
}

type App struct {
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
	inspectors     *inspector.Registry
	gizmos         *gizmo.Registry
	hierarchy      *hierarchy.Panel
	sceneView      *sceneview.Panel
	assetsPanel    *assets.Panel
	thumbs         *assets.Thumbs
	gameView       *gameview.Panel
	profiler       *profiler.Panel
	consoleSink    *console.Sink
	play           *play.Controller
	ext            *extension.Registry
	restart        bool
	sessionAt      float64
	buildSeen      time.Time
	lastInput      float64
	lastFrame      time.Time
	buildState     widgets.DotState
	consolePanel   *console.Panel
	inspectorPanel *inspector.Panel
	scriptLog      *log.ChannelLogger
	problemRev     uint64
	scriptsChanged bool
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

func (a *App) Play() *play.Controller {
	return a.play
}

func (a *App) Extensions() *extension.Registry {
	return a.ext
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
	r := a.opt.Renderer
	ui := core.New(r.UIFont())
	ui.Theme = theme.Editor()
	if set, err := text.LoadSet(e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetFonts(set)
	} else {
		a.log.Warn("ui fonts are missing, using the debug font", log.F("err", err.Error()))
	}
	clip := clipboard{win: e.Window()}
	ui.Clipboard = clip
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
	a.ctx = &context.Editor{
		Engine:      e,
		Renderer:    r,
		Project:     p,
		Pipeline:    a.opt.Pipeline,
		Loader:      a.opt.Loader,
		Types:       a.opt.Types,
		Docs:        document.NewManager(document.Options{Root: p.Root, Types: a.opt.Types, Resolver: res, Log: lg}),
		Selection:   selection.New(),
		Commands:    command.New(),
		UI:          ui,
		DnD:         &dnd.State{},
		Clipboard:   clip,
		Log:         a.log,
		Settings:    context.DefaultSettings(),
		RenderStats: a.opt.RenderStats,
		Scripts:     a.opt.Scripts,
		Collapsed:   map[string]bool{},
	}
	a.ctx.Commands.OnError = func(id string, err error) {
		a.ctx.Notify(context.LevelError, err.Error())
	}
	a.ctx.DnD.Trace = func(event string, p dnd.Payload) {
		a.log.Debug("dnd "+event, log.F("kind", p.Kind), log.F("label", p.Label))
	}

	a.templates = templates.New()
	if err := templates.RegisterBuiltin(a.templates); err != nil {
		return err
	}
	a.inspectors = inspector.NewRegistry()
	if err := inspector.RegisterBuiltin(a.inspectors, a.opt.Types.Schema()); err != nil {
		return err
	}
	a.gizmos = gizmo.NewRegistry()
	a.play = play.New(a.ctx)
	a.ctx.Play = a.play
	a.play.Subscribe(func(st play.State) {
		if st == play.Stopped {
			a.scriptsChanged = false
			for _, d := range a.ctx.Docs.Docs() {
				a.ctx.PruneSelectionOf(d.ID)
			}
		}
	})
	a.thumbs = assets.NewThumbs(a.ctx)
	a.ctx.Thumbs = a.thumbs
	a.hierarchy = hierarchy.New(a.templates)
	a.sceneView = sceneview.New(a.gizmos)
	a.sceneView.Templates = a.templates
	a.assetsPanel = assets.New(a.thumbs)
	a.gameView = gameview.New(a.play)
	a.profiler = profiler.New()

	a.ws = workspace.New(p.Root, a.log)
	for _, pn := range a.opt.Panels {
		if err := a.ws.Register(pn); err != nil {
			return err
		}
	}
	a.consolePanel = console.New(a.consoleSink)
	a.inspectorPanel = inspector.New(a.inspectors)
	builtins := []workspace.Panel{
		a.hierarchy,
		a.inspectorPanel,
		a.sceneView,
		a.gameView,
		a.assetsPanel,
		a.consolePanel,
		a.profiler,
	}
	for _, pn := range builtins {
		if !a.ws.Has(pn.ID()) {
			if err := a.ws.Register(pn); err != nil {
				return err
			}
		}
	}
	a.scriptLog = lg.With(log.Channel("script"))
	a.ctx.ShowProblems = func(filter string) {
		a.ws.Show(workspace.Console)
		a.consolePanel.Focus(filter)
	}
	a.ctx.RevealPath = func(rel string) {
		a.ws.Show(workspace.Assets)
		a.assetsPanel.Reveal(a.ctx, rel)
	}
	a.consolePanel.OnOpen = a.ctx.RevealPath
	if err := a.registerCommands(); err != nil {
		return err
	}
	a.ext = extension.New(extension.Deps{
		Editor:     a.ctx,
		Workspace:  a.ws,
		Inspectors: a.inspectors,
		Gizmos:     a.gizmos,
		Templates:  a.templates,
		Commands:   a.ctx.Commands,
		Schema:     a.opt.Types.Schema(),
	})
	for _, m := range a.opt.Extensions {
		if err := a.ext.Load(m); err != nil {
			a.ctx.Notify(context.LevelError, "extension "+extension.ModuleID(m)+" failed to register: "+err.Error())
		}
	}
	if err := a.registerWindowCommands(); err != nil {
		return err
	}
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
	case platform.EvTextEditing:
		a.ui.SetComposition(ev.Str, int(ev.EditStart))
	case platform.EvTextInput:
		a.ui.SetComposition("", 0)
	case platform.EvWindowFocusGained:
		a.checkAt = 0
	case platform.EvWindowFocusLost:
		a.gameView.Release()
	case platform.EvDropFile:
		a.drops = append(a.drops, ev.Str)
		a.dropPos = ev.Mouse.Pos
	case platform.EvDropComplete:
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
	in := e.Input()
	if !ui.ModalActive() && ui.Focus() == 0 && !a.ws.Dragging() && !a.ctx.DnD.Active() && !a.play.Captured() {
		a.ctx.Commands.Dispatch(in, a.ws.Scope(a.ws.Hovered()))
	}
	m := e.Metrics()
	size := mathx.Vec2{X: float32(m.LogicalW), Y: float32(m.LogicalH)}
	ui.Begin(in, size, m.Density())
	a.ctx.FocusedPanel = a.ws.Focused()
	th := &ui.Theme
	bh := th.BarHeight
	menuR := mathx.Rect{W: size.X, H: bh}
	toolR := mathx.Rect{Y: bh, W: size.X, H: bh}
	bottom := mathx.Rect{Y: size.Y - th.MenuHeight, W: size.X, H: th.MenuHeight}
	body := mathx.Rect{Y: 2 * bh, W: size.X, H: max(0, size.Y-2*bh-bottom.H)}
	ui.Rect(mathx.Rect{W: size.X, H: size.Y}, th.Background)
	a.ws.Draw(a.ctx, body)
	if a.play.State() != play.Stopped {
		l := ui.List()
		l.PushLayer(draw.LayerTop)
		l.RectOutline(body, th.PlayTint, 2)
		l.PopLayer()
	}
	a.drawScriptsBanner(body)
	a.drawStatus(bottom)
	a.drawToolbar(toolR)
	a.drawMenuBar(menuR)
	for _, q := range a.ctx.TakeQuestions() {
		a.modals = append(a.modals, modal{kind: modalAsk, ask: q})
	}
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

func (a *App) idle() bool {
	now := a.eng.Platform().Now()
	return a.play.State() == play.Stopped &&
		now-a.lastInput > 1 &&
		!a.ctx.DnD.Active() &&
		!a.ws.Dragging() &&
		!a.thumbs.Busy() &&
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
	first := a.problemRev == 0
	a.problemRev = rev
	if !first && a.play.State() != play.Stopped {
		a.scriptsChanged = true
	}
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
	}
}

func (a *App) restartPlay() {
	a.stopPlay()
	if err := a.togglePlay(); err != nil {
		a.ctx.Notify(context.LevelError, err.Error())
	}
}

func (a *App) drawScriptsBanner(body mathx.Rect) {
	if !a.scriptsChanged || a.play.State() == play.Stopped {
		return
	}
	ui := a.ui
	th := &ui.Theme
	h := th.RowHeight + 8
	w := min(body.W-16, 460)
	r := mathx.Rect{X: body.X + (body.W-w)/2, Y: body.Y + 6, W: w, H: h}
	ui.Rect(r, th.PanelAlt)
	ui.Outline(r, th.Warn)
	ui.PushClip(r)
	ui.Text("Scripts changed — restart Play to apply", mathx.Vec2{X: r.X + th.Padding, Y: ui.TextY(r)}, th.Warn)
	ui.PopClip()
	br := mathx.Rect{X: r.X + r.W - 84, Y: r.Y + 4, W: 78, H: h - 8}
	if widgets.ButtonRect(ui, ui.ID("scripts.restart"), br, "Restart") {
		a.restartPlay()
	}
}

func (a *App) periodic(now float64) {
	a.pollSession(now)
	if a.restart {
		return
	}
	a.reportScripts()
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
	if t != a.title {
		a.title = t
		a.eng.Window().SetTitle(t)
	}
}
