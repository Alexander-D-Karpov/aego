//go:build aego_editor

package docsview

import (
	"aego/app/assembly"
	playmods "aego/app/assembly/playground"
	"aego/asset"
	"aego/asset/loader"
	"aego/core/log"
	"aego/core/mathx"
	edclip "aego/editor/core/clipboard"
	"aego/editor/core/context"
	"aego/editor/core/docsession"
	"aego/editor/core/services"
	"aego/editor/ext/inspect"
	"aego/editor/panels/gameview"
	"aego/editor/panels/inspector"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/format/aetext"
	scriptsystems "aego/modules/scripting/systems"
	"aego/platform"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/icons"
	"aego/render/text"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/script/bind"
	"aego/ui/core"
	"aego/ui/draw"
	"aego/ui/theme"
	"aego/ui/widgets"
)

const (
	appID      = "aego-docs"
	maxSprites = 16384
	gameShare  = 0.6
	stackBelow = 640
	maxLines   = 200
	defaultW   = 960
	defaultH   = 440
)

var coreBlocks = []string{"window", "defaults"}

type Options struct {
	Headless  bool
	FrameTime float64
	Width     int
	Height    int
}

type Line struct {
	Text  string `json:"text"`
	Error bool   `json:"error"`
}

type App struct {
	OnChange func(text string)
	opt      Options
	types    *scene.Types
	man      *module.Manifest
	e        *engine.Engine
	renderer *render.Renderer
	loader   *loader.Loader
	ui       *core.Context
	clip     *docsession.Clipboard
	session  *docsession.Session
	insp     *inspector.Panel
	game     *gameview.Panel
	list     *draw.List
	log      *log.Logger
	lines    []Line
	textOn   bool
	closed   bool
}

func noBlock(string) *aetext.Node {
	return nil
}

func New(opt Options) (*App, error) {
	if opt.Width <= 0 || opt.Height <= 0 {
		opt.Width, opt.Height = defaultW, defaultH
	}
	a := &App{opt: opt, clip: &docsession.Clipboard{}}
	a.log = log.New(log.FuncSink(a.record))
	a.log.SetLevel(log.Info)
	var err error
	if a.types, err = assembly.NewTypes(); err != nil {
		return nil, err
	}
	reg := bind.New()
	if a.man, err = playmods.Catalog().Load(a.types, reg, assembly.Options{Seed: 1}); err != nil {
		return nil, err
	}
	sections, err := a.man.LoadSections(noBlock, coreBlocks...)
	if err != nil {
		return nil, err
	}
	if err := a.types.Schema().Freeze(); err != nil {
		return nil, err
	}
	if err := scriptsystems.Freeze(reg, a.types.Schema()); err != nil {
		return nil, err
	}
	rmod := renderhost.New(render.Options{MaxSprites: maxSprites})
	b := engine.NewBuilder(engine.Config{
		Title:             "Aego",
		Width:             opt.Width,
		Height:            opt.Height,
		Headless:          opt.Headless,
		HeadlessFrameTime: opt.FrameTime,
		VSync:             platform.VSyncOn,
		AppID:             appID,
		Seed:              1,
		LoopScope:         engine.ScopeEditor,
	}).WithLogger(a.log).Use(rmod)
	if err := frame.Register(b.Registry()); err != nil {
		return nil, err
	}
	if ts, ok := module.SectionValue[base.Settings](sections, base.Section); ok {
		b.Configure(ts.Apply)
	}
	b.Use(a.man.Engine())
	if a.e, err = b.Build(); err != nil {
		return nil, err
	}
	a.renderer = rmod.Renderer()
	a.loader = loader.New(a.e.GPU(), a.renderer.Textures(), a.log)
	if err := a.loader.Mount(asset.NewIndex(), a.e.VFS()); err != nil {
		a.Close()
		return nil, err
	}
	a.e.SetService(loader.ServiceID, a.loader)
	a.e.SetService(renderhost.ServiceID, a.renderer)
	if err := a.man.Init(&module.InitContext{Engine: a.e, Assets: a.loader, Log: a.log, Editor: true, Sections: sections}); err != nil {
		a.Close()
		return nil, err
	}
	a.initUI()
	a.session = docsession.New(docsession.Options{
		Engine: a.e, Renderer: a.renderer, Loader: a.loader, Types: a.types, Sections: sections,
		Scripts:   &scriptHost{reg: reg, types: a.types, man: a.man, log: a.log},
		UI:        a.ui,
		Clipboard: a.clip,
		Log:       a.log,
		OnChange: func(t []byte) {
			if a.OnChange != nil {
				a.OnChange(string(t))
			}
		},
	})
	return a, a.initPanels()
}

func (a *App) initUI() {
	r := a.renderer
	ui := core.New(r.UIFont())
	ui.Theme = theme.Editor()
	ui.Clipboard = a.clip
	if sh, err := draw.NewShapes(a.e.GPU(), r.Textures()); err == nil {
		ui.SetShapes(sh)
	}
	if set, err := text.LoadSet(a.e.VFS(), r.Textures(), "engine://fonts", "ui", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetFonts(set)
	}
	if set, err := text.LoadSet(a.e.VFS(), r.Textures(), "engine://fonts", "ui-mono", []int{12, 14, 16, 20}, []int{1, 2}); err == nil {
		ui.SetMonoFonts(set)
	}
	if set, err := icons.Load(a.e.VFS(), r.Textures(), "engine://icons"); err == nil {
		ui.Icons = set
	}
	a.ui = ui
}

func (a *App) initPanels() error {
	ctx := a.session.Editor()
	reg := inspect.NewRegistry()
	if err := reg.Load(a.types.Schema()); err != nil {
		return err
	}
	reg.SetEditor(ctx)
	ids := make([]string, len(a.man.Modules))
	for i, m := range a.man.Modules {
		ids[i] = m.ID
	}
	reg.SetModules(ids)
	a.insp = inspector.New(reg, inspector.Deps{
		Docs:      a.session,
		Selection: a.session,
		Commands:  ctx.Commands,
		Notices:   ctx,
		Nav:       ctx,
		Problems:  ctx,
		Sections:  ctx.Sections,
		Clip:      edclip.EnvOf(ctx),
		Assets:    docsession.Unavailable{},
		Scenes:    docsession.Unavailable{},
		Collapsed: ctx.Collapsed,
		Schema:    a.types.Schema(),
	})
	e := a.e
	a.game = gameview.New(a.session.Controller(), &services.Viewport{
		UI:       a.ui,
		GPU:      e.GPU(),
		Renderer: a.renderer,
		Shapes:   ctx.Shapes,
		Overlay:  &ctx.Overlay,
		OnRender: ctx.OnRender,
		RenderWorld: func(w *engine.World) {
			e.RunPhase(w, engine.PhaseRenderPrepare, engine.ScopeEditor, 0, 0)
			e.RunPhase(w, engine.PhaseRender, engine.ScopeEditor, 0, 0)
		},
	})
	return nil
}

func (a *App) record(r log.Record) {
	if len(a.lines) >= maxLines {
		a.lines = append(a.lines[:0], a.lines[len(a.lines)-maxLines/2:]...)
	}
	a.lines = append(a.lines, Line{Text: r.Msg, Error: r.Level > log.Info})
}

func (a *App) Session() *docsession.Session {
	return a.session
}

func (a *App) Run() error {
	return a.e.Run(a)
}

func (a *App) Step(n int) error {
	for i := 0; i < n; i++ {
		if !a.e.Step(a) {
			return a.e.Err()
		}
	}
	return nil
}

func (a *App) Init(*engine.Engine) error {
	return nil
}

func (a *App) Event(ev platform.Event) {
	switch ev.Kind {
	case platform.EvTextEditing:
		a.ui.SetComposition(ev.Str, int(ev.EditStart))
	case platform.EvTextInput:
		a.ui.SetComposition("", 0)
	case platform.EvWindowFocusLost:
		a.game.Release()
	}
}

func (a *App) FixedUpdate(float64) {}

func layout(body mathx.Rect, gap float32) (game, insp mathx.Rect) {
	if body.W < stackBelow {
		h := body.H * gameShare
		return mathx.Rect{X: body.X, Y: body.Y, W: body.W, H: h},
			mathx.Rect{X: body.X, Y: body.Y + h + gap, W: body.W, H: max(0, body.H-h-gap)}
	}
	w := body.W * gameShare
	return mathx.Rect{X: body.X, Y: body.Y, W: w, H: body.H},
		mathx.Rect{X: body.X + w + gap, Y: body.Y, W: max(0, body.W-w-gap), H: body.H}
}

func (a *App) panel(f *services.Frame, r mathx.Rect, drawFn func(*services.Frame, mathx.Rect)) {
	ui := a.ui
	ui.Region(r)
	ui.PushClip(r)
	drawFn(f, r)
	ui.PopClip()
	ui.PopCursor()
}

func (a *App) toolbar(r mathx.Rect) {
	ui := a.ui
	st := a.session.State()
	label, tip := "Pause", "Pause the simulation"
	if !st.Running || st.Paused {
		label, tip = "Play", "Run the simulation"
	}
	tb := widgets.BeginToolbar(ui, r)
	if c, _ := tb.TextButton(ui.ID("play"), label, tip); c {
		a.session.TogglePause()
	}
	if c, _ := tb.TextButton(ui.ID("reset"), "Reset", "Load the original scene again"); c {
		if err := a.session.Reset(); err != nil {
			a.session.Editor().Notify(context.LevelError, err.Error())
		}
	}
	if c, _ := tb.TextButton(ui.ID("undo"), "Undo", "Undo the last change"); c && st.CanUndo {
		a.session.Undo()
	}
	if c, _ := tb.TextButton(ui.ID("redo"), "Redo", "Redo the change"); c && st.CanRedo {
		a.session.Redo()
	}
}

func (a *App) Update(dt float64) {
	e := a.e
	now := e.Platform().Now()
	a.session.Tick(dt)
	ui := a.ui
	ui.Time = now
	m := e.Metrics()
	size := mathx.Vec2{X: float32(m.LogicalW), Y: float32(m.LogicalH)}
	ui.Begin(e.Input(), size, m.Density())
	th := &ui.Theme
	ui.Rect(mathx.Rect{W: size.X, H: size.Y}, th.Background)
	ctx := a.session.Editor()
	frame := services.Frame{UI: ui, DnD: ctx.DnD, Now: now, Focused: true}
	bar := mathx.Rect{W: size.X, H: th.BarHeight}
	a.toolbar(bar)
	body := mathx.Rect{Y: bar.H, W: size.X, H: max(0, size.Y-bar.H)}
	gameR, inspR := layout(body, 1)
	a.panel(&frame, gameR, a.game.Draw)
	a.panel(&frame, inspR, a.insp.Draw)
	ctx.DnD.Frame(ui)
	a.list = ui.End()
	a.applyPlatform()
}

func (a *App) applyPlatform() {
	w := a.e.Window()
	if ti, ok := w.(platform.TextInputer); ok {
		want, r, cur := a.ui.WantsTextInput()
		if want != a.textOn {
			if want {
				ti.StartTextInput()
			} else {
				ti.StopTextInput()
			}
			a.textOn = want
		}
		if want {
			ti.SetTextInputArea(r, cur)
		}
	}
	if c, ok := w.(platform.Cursorer); ok {
		c.SetCursor(platform.CursorShape(a.ui.CursorShape()))
	}
}

func (a *App) Render(float64) {
	a.session.Editor().RunRenderHooks()
	if a.list == nil || a.list.Empty() {
		return
	}
	m := a.e.Metrics()
	v := view.ScreenView(m.PixelW, m.PixelH, m.Density())
	v.Clear = a.ui.Theme.Background
	v.DoClear = true
	v.Label = "docs.ui"
	white := a.renderer.White()
	if f := a.ui.Font(); f != nil && !f.White.IsZero() {
		white = f.White
	}
	f := a.renderer.View(v)
	a.list.Flush(f, a.ui.Font(), white, 0)
	f.End()
}

func (a *App) Shutdown() {
	a.session.Close()
	a.game.Close()
}

func (a *App) Close() {
	if a.closed {
		return
	}
	a.closed = true
	if a.session != nil {
		a.session.Close()
	}
	if a.game != nil {
		a.game.Close()
	}
	a.man.Shutdown()
	if a.e != nil {
		a.e.Close()
	}
	if a.loader != nil {
		a.loader.Close()
	}
}
