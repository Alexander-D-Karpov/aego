package playground

import (
	"fmt"
	"strings"

	playmods "aego/app/assembly/playground"

	"aego/asset"
	"aego/asset/loader"
	"aego/core/log"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/base"
	"aego/engine/module"
	"aego/engine/recorder"
	"aego/platform"
	"aego/render"
	renderhost "aego/render/host"
	"aego/render/material"
	"aego/render/sprite"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/script/bind"
	"aego/script/bytecode"
	scriptruntime "aego/script/runtime"
)

const (
	maxLines    = 500
	gameWidth   = 960
	gameHeight  = 540
	maxSprites  = 16384
	statsAlpha  = 0.1
	benchLabel  = "playground.bench"
	benchName   = "bench"
	gameAppID   = "aego-playground"
	recordDepth = 1024
)

type GameConfig struct {
	Headless  bool
	FrameTime float64
}

type Stats struct {
	Running   bool    `json:"running"`
	Example   string  `json:"example,omitempty"`
	Frames    uint64  `json:"frames"`
	FrameMs   float64 `json:"frameMs"`
	Calls     uint64  `json:"calls"`
	DrawCalls uint32  `json:"drawCalls"`
	Sprites   uint32  `json:"sprites"`
	Lost      bool    `json:"lost"`
	Restores  int     `json:"restores"`
	Renderer  string  `json:"renderer"`
}

type Poll struct {
	Log    []Line  `json:"log"`
	Faults []Fault `json:"faults"`
	Stats  Stats   `json:"stats"`
}

type gpuCalls interface {
	Calls() uint64
}

type gpuLoss interface {
	Lost() bool
	Restores() int
	SimulateLoss(lose bool) bool
}

type bench struct {
	count   int
	animate bool
	t       float64
	mat     *material.Material
}

type Game struct {
	cfg      GameConfig
	types    *scene.Types
	reg      *bind.Registry
	man      *module.Manifest
	e        *engine.Engine
	renderer *render.Renderer
	rstats   func() render.Stats
	loader   *loader.Loader
	runner   *frame.Runner
	rt       *scriptruntime.Runtime
	scene    *scene.Scene
	detach   func()
	log      *log.Logger
	clog     *log.ChannelLogger
	lines    []Line
	faults   int
	example  string
	frames   uint64
	frameMs  float64
	bench    bench
	running  bool
	looping  bool
	overflow bool
}

type driver struct {
	g *Game
}

func (d driver) Init(*engine.Engine) error { return nil }
func (d driver) Event(platform.Event)      {}
func (d driver) FixedUpdate(float64)       {}
func (d driver) Update(dt float64)         { d.g.update(dt) }
func (d driver) Render(float64)            { d.g.render() }
func (d driver) Shutdown()                 {}

func newGame(cfg GameConfig, types *scene.Types, reg *bind.Registry, man *module.Manifest, sections *module.Sections) (*Game, error) {
	g := &Game{cfg: cfg, types: types, reg: reg, man: man, runner: frame.New()}
	g.log = log.New(log.FuncSink(g.record))
	g.log.SetLevel(log.Info)
	g.clog = g.log.With(log.Engine)
	rmod := renderhost.New(render.Options{MaxSprites: maxSprites})
	b := engine.NewBuilder(engine.Config{
		Title:             "Aego Playground",
		Width:             gameWidth,
		Height:            gameHeight,
		Headless:          cfg.Headless,
		HeadlessFrameTime: cfg.FrameTime,
		VSync:             platform.VSyncOn,
		AppID:             gameAppID,
		Seed:              1,
	}).WithLogger(g.log).Use(rmod)
	if err := frame.Register(b.Registry()); err != nil {
		return nil, err
	}
	if ts, ok := module.SectionValue[base.Settings](sections, base.Section); ok {
		b.Configure(ts.Apply)
	}
	b.Use(man.Engine())
	e, err := b.Build()
	if err != nil {
		return nil, err
	}
	g.e = e
	g.renderer = rmod.Renderer()
	g.rstats = rmod.Stats
	g.loader = loader.New(e.GPU(), g.renderer.Textures(), g.log)
	g.loader.SetMaterials(g.renderer.Materials())
	if err := g.loader.Mount(asset.NewIndex(), e.VFS()); err != nil {
		e.Close()
		return nil, err
	}
	e.SetService(loader.ServiceID, g.loader)
	playmods.UseMemorySaves(e)
	e.SetService(renderhost.ServiceID, g.renderer)
	e.SetView(frame.ServiceID, g.runner)
	e.MainWorld().Recorder = recorder.New(recordDepth)
	if err := man.Init(&module.InitContext{Engine: e, Assets: g.loader, Log: g.log, Sections: sections}); err != nil {
		e.Close()
		return nil, err
	}
	return g, nil
}

func (g *Game) record(r log.Record) {
	if strings.HasPrefix(r.Msg, "script stopped: ") {
		return
	}
	if len(g.lines) >= maxLines {
		g.lines = append(g.lines[:0], g.lines[len(g.lines)-maxLines/2:]...)
	}
	g.lines = append(g.lines, record(&g.frames, r))
}

func (g *Game) start() {
	if g.looping || g.cfg.Headless {
		return
	}
	g.looping = true
	go func() {
		if err := g.e.Run(driver{g: g}); err != nil {
			g.clog.Error("the game loop stopped", log.F("err", err.Error()))
		}
	}()
}

func (g *Game) Step(n int) error {
	for i := 0; i < n; i++ {
		if !g.e.Step(driver{g: g}) {
			return g.e.Err()
		}
	}
	return nil
}

func (g *Game) Engine() *engine.Engine {
	return g.e
}

// Scene is the scene Play loaded, nil before the first Play or after Stop.
func (g *Game) Scene() *scene.Scene {
	return g.scene
}

func (g *Game) Play(name string, src []byte, mods []*bytecode.Module) error {
	g.Stop()
	rt := scriptruntime.New(scriptruntime.Options{Bindings: g.reg, Schema: g.types.Schema(), Log: g.log, VM: playmods.SandboxVM()})
	if err := rt.Link(mods); err != nil {
		rt.Close()
		return err
	}
	s, rep, err := sceneio.Load(src, ScenePrefix+name+sceneExt, g.types, g.loader, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		rt.Close()
		return err
	}
	for _, w := range rep.Warnings {
		g.clog.Warn("scene warning", log.F("path", w.Path), log.F("line", w.Pos.Line), log.F("msg", w.Msg))
	}
	for _, u := range rep.Unknown {
		g.clog.Warn("unknown component preserved", log.F("type", u))
	}
	g.rt, g.scene = rt, s
	g.e.SetService(scriptruntime.ServiceID, rt)
	g.e.SetService(scene.ServiceID, s)
	g.runner.Bind(s, g.renderer, g.loader)
	if err := rt.Instantiate(s); err != nil {
		g.Stop()
		return err
	}
	d, err := g.man.Attach(&module.AttachContext{World: g.e.MainWorld(), Scene: s, Log: g.log, Play: true})
	if err != nil {
		g.Stop()
		return err
	}
	g.detach = d
	g.example, g.running, g.faults, g.overflow = name, true, 0, false
	g.start()
	return nil
}

func (g *Game) Bench(count int, animate, withMaterial bool) error {
	g.Stop()
	g.bench = bench{count: min(max(count, 1), maxSprites), animate: animate}
	if withMaterial {
		m, err := BenchMaterial()
		if err != nil {
			return err
		}
		g.bench.mat = m
		g.renderer.PinClock(&BenchClock)
	}
	g.example, g.running = benchName, true
	g.start()
	return nil
}

func (g *Game) Stop() {
	if g.detach != nil {
		g.detach()
		g.detach = nil
	}
	w := g.e.MainWorld()
	if g.rt != nil {
		g.rt.Close()
		g.rt = nil
		w.RemoveService(scriptruntime.ServiceID)
	}
	if g.scene != nil {
		g.scene = nil
		g.runner.SetScene(nil)
		w.RemoveService(scene.ServiceID)
	}
	g.bench = bench{}
	g.renderer.PinClock(nil)
	g.running = false
	g.example = ""
}

func (g *Game) LoseContext(lose bool) bool {
	l, ok := g.e.GPU().(gpuLoss)
	return ok && l.SimulateLoss(lose)
}

func (g *Game) update(dt float64) {
	g.frames++
	ft := g.e.Stats().FrameTime * 1000
	if g.frameMs == 0 {
		g.frameMs = ft
	} else {
		g.frameMs += (ft - g.frameMs) * statsAlpha
	}
	g.bench.t += dt
	if g.scene != nil && !g.overflow && g.scene.Stats().Alive > playmods.SandboxNodes {
		g.overflow = true
		g.clog.Error(fmt.Sprintf("the scene grew past %d nodes, the game was stopped", playmods.SandboxNodes))
	}
}

func hue(i, n int) mathx.Color {
	h := float32(i) / float32(max(n, 1)) * 6
	f := h - float32(int(h))
	q, t := 1-f, f
	switch int(h) % 6 {
	case 0:
		return mathx.Color{R: 1, G: t, A: 1}
	case 1:
		return mathx.Color{R: q, G: 1, A: 1}
	case 2:
		return mathx.Color{G: 1, B: t, A: 1}
	case 3:
		return mathx.Color{G: q, B: 1, A: 1}
	case 4:
		return mathx.Color{R: t, B: 1, A: 1}
	}
	return mathx.Color{R: 1, B: q, A: 1}
}

func (g *Game) render() {
	b := &g.bench
	if b.count == 0 || g.renderer == nil {
		return
	}
	m := g.e.Metrics()
	if m.PixelW <= 0 || m.PixelH <= 0 {
		return
	}
	scale := m.Density()
	v := view.ScreenView(m.PixelW, m.PixelH, scale)
	v.DoClear, v.Clear, v.Label = true, mathx.Color{R: 0.07, G: 0.08, B: 0.1, A: 1}, benchLabel
	f := g.renderer.View(v)
	w, h := float32(m.PixelW)/scale, float32(m.PixelH)/scale
	cols := max(1, int(mathx.Sqrt(float32(b.count)*w/h)+0.5))
	rows := (b.count + cols - 1) / cols
	cw, ch := w/float32(cols), h/float32(rows)
	size := min(cw, ch) * 0.7
	white := g.renderer.White()
	for i := 0; i < b.count; i++ {
		var rot float32
		if b.animate {
			rot = float32(b.t)*1.5 + float32(i)*0.05
		}
		f.Sprite(sprite.Draw{
			Region:   white,
			Position: mathx.Vec2{X: (float32(i%cols) + 0.5) * cw, Y: (float32(i/cols) + 0.5) * ch},
			Size:     mathx.Vec2{X: size, Y: size},
			Origin:   mathx.Vec2{X: 0.5, Y: 0.5},
			Rotation: rot,
			Tint:     hue(i, b.count),
			Material: b.mat,
		})
	}
	f.End()
}

func (g *Game) newFaults() []Fault {
	if g.rt == nil {
		return nil
	}
	all := g.rt.Faults()
	if len(all) < g.faults {
		g.faults = 0
	}
	var out []Fault
	for _, f := range all[g.faults:] {
		x := Fault{Script: f.Type, Node: f.Path, Code: f.Fault.Code, Message: f.Fault.Message}
		if len(f.Fault.Stack) > 0 {
			x.File = strings.TrimPrefix(f.Fault.Stack[0].File, ScriptsDir+"/")
			x.Line = int(f.Fault.Stack[0].Line)
		}
		out = append(out, x)
	}
	g.faults = len(all)
	return out
}

func (g *Game) Poll() Poll {
	if g.overflow {
		g.overflow = false
		g.Stop()
	}
	p := Poll{Log: g.lines, Faults: g.newFaults()}
	g.lines = nil
	s := &p.Stats
	s.Running, s.Example, s.Frames, s.FrameMs = g.running, g.example, g.frames, g.frameMs
	s.DrawCalls = g.e.Stats().DrawCalls
	s.Renderer = g.e.Caps().Renderer
	if g.rstats != nil {
		s.Sprites = g.rstats().Instances
	}
	if c, ok := g.e.GPU().(gpuCalls); ok {
		s.Calls = c.Calls()
	}
	if l, ok := g.e.GPU().(gpuLoss); ok {
		s.Lost, s.Restores = l.Lost(), l.Restores()
	}
	return p
}

func (g *Game) Close() {
	g.Stop()
	g.man.Shutdown()
	g.runner.Close()
	g.e.Close()
	g.loader.Close()
}
