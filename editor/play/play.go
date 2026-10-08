package play

import (
	"fmt"
	"strconv"
	"time"

	"aego/asset"
	"aego/core/clock"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/document"
	"aego/editor/history"
	"aego/editor/history/commands"
	"aego/engine"
	"aego/platform/input"
	"aego/render/rt"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/systems"
	"aego/schema"
)

type State uint8

const (
	Stopped State = iota
	Playing
	Paused
)

type Stats struct {
	Update  float64
	Render  float64
	Nodes   int
	Sprites int
	Views   int
}

type Controller struct {
	ctx      *context.Editor
	state    State
	world    *engine.World
	runner   *systems.Runner
	scene    *scene.Scene
	doc      document.ID
	stepper  clock.FixedStepper
	stepOnce bool
	input    input.State
	captured bool
	origin   mathx.Vec2
	scale    float32
	frames   uint64
	simTime  float64
	stats    Stats
	proxy    *document.SceneDocument
	subs     []stateSub
	nextSub  uint64
	detach   func()
}

func New(ctx *context.Editor) *Controller {
	return &Controller{ctx: ctx, scale: 1}
}

func (c *Controller) State() State {
	return c.state
}

func (c *Controller) Stats() Stats {
	return c.stats
}

func (c *Controller) Scene() *scene.Scene {
	return c.scene
}

func (c *Controller) World() *engine.World {
	return c.world
}

func (c *Controller) Doc() document.ID {
	return c.doc
}

func (c *Controller) Frames() uint64 {
	return c.frames
}

func (c *Controller) SimTime() float64 {
	return c.simTime
}

func (c *Controller) resolver() asset.Resolver {
	if c.ctx.Loader != nil {
		return c.ctx.Loader
	}
	return nil
}

type stateSub struct {
	id uint64
	fn func(State)
}

func (c *Controller) Subscribe(fn func(State)) func() {
	c.nextSub++
	id := c.nextSub
	c.subs = append(c.subs, stateSub{id: id, fn: fn})
	return func() {
		for i := range c.subs {
			if c.subs[i].id == id {
				c.subs = append(c.subs[:i], c.subs[i+1:]...)
				return
			}
		}
	}
}

func (c *Controller) notify() {
	for i := range c.subs {
		c.subs[i].fn(c.state)
	}
}

func (c *Controller) PlayDoc() *document.SceneDocument {
	return c.proxy
}

func (c *Controller) History() *history.History {
	if c.proxy == nil {
		return nil
	}
	return c.proxy.History
}

func (c *Controller) Start(d *document.SceneDocument) error {
	if c.ctx.Scripts != nil {
		if n := c.ctx.Scripts.Errors(); n > 0 {
			return errs.New(errs.EditorCommand, strconv.Itoa(n)+" script package error(s), fix them before Play")
		}
	}
	if c.state != Stopped {
		c.Stop()
	}
	real := c.ctx.Docs.Get(d.ID)
	if real == nil {
		return errs.New(errs.EditorDocument, "scene is not open")
	}
	if err := c.start(sceneio.Save(real.Scene()), real.Path); err != nil {
		return err
	}
	c.doc = real.ID
	c.proxy = real.PlayCopy(c.scene)
	c.ctx.Docs.SetPlay(c.proxy, c.world)
	c.notify()
	return nil
}

func (c *Controller) StartProject() error {
	ctx := c.ctx
	p := ctx.Project
	if p == nil || p.MainScene.IsZero() || ctx.Pipeline == nil {
		return errs.New(errs.EditorDocument, "project has no main scene")
	}
	rel, ok := ctx.Pipeline.PathOf(p.MainScene)
	if !ok {
		return errs.New(errs.EditorDocument, "main scene is not in the asset pipeline")
	}
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		return err
	}
	return c.Start(d)
}

func (c *Controller) start(data []byte, name string) error {
	ctx := c.ctx
	res := c.resolver()
	s, report, err := sceneio.Load(data, name, ctx.Types, res, sceneio.LoadOptions{KeepIDs: true})
	if err != nil {
		return errs.Wrap(err, errs.EditorDocument, "cannot clone the scene for play mode")
	}
	for _, u := range report.Unknown {
		ctx.Notify(context.LevelWarn, "component "+u+" is not loaded in this build and does nothing in play mode")
	}
	c.scene = s
	c.world = engine.NewWorld("play")
	c.world.Input = &c.input
	c.runner = systems.New()
	c.runner.Bind(s, ctx.Renderer, res)
	c.world.SetService(systems.RunnerServiceID, c.runner)
	c.world.SetService(scene.ServiceID, s)
	cfg := ctx.Engine.Config()
	c.stepper = clock.FixedStepper{
		FixedDelta:    1 / cfg.FixedHz,
		MaxFrameDelta: cfg.MaxFrameDelta,
		MaxSteps:      cfg.MaxFixedSteps,
	}
	c.input = input.State{}
	c.frames, c.simTime = 0, 0
	c.stepOnce = false
	if ctx.Scripts != nil {
		detach, err := ctx.Scripts.Attach(c.world, s)
		if err != nil {
			c.world, c.runner, c.scene = nil, nil, nil
			return errs.Wrap(err, errs.EditorDocument, "cannot start the scripts")
		}
		c.detach = detach
	}
	c.state = Playing
	ctx.Notify(context.LevelInfo, "Play mode started")
	return nil
}

func (c *Controller) Pause() {
	switch c.state {
	case Playing:
		c.state = Paused
	case Paused:
		c.state = Playing
	default:
		return
	}
	c.notify()
}

func (c *Controller) Step() {
	if c.state != Paused {
		return
	}
	c.stepOnce = true
}

func (c *Controller) Stop() {
	if c.state == Stopped {
		return
	}
	if real := c.ctx.Docs.Get(c.doc); real != nil && c.proxy != nil {
		real.View.Camera = c.proxy.View.Camera
		real.View.Mode = c.proxy.View.Mode
		real.View.Orbit = c.proxy.View.Orbit
	}
	if c.detach != nil {
		c.detach()
		c.detach = nil
	}
	c.ctx.Docs.ClearPlay()
	c.proxy = nil
	c.state = Stopped
	c.world, c.runner, c.scene = nil, nil, nil
	c.doc = 0
	c.captured = false
	c.input = input.State{}
	c.stats = Stats{}
	c.notify()
	c.ctx.Notify(context.LevelInfo, "Play mode stopped")
}

func (c *Controller) CanCopyToEdit(nodes []ids.NodeID, t schema.TypeID) bool {
	if c.state == Stopped || c.scene == nil || len(nodes) == 0 {
		return false
	}
	real := c.ctx.Docs.Get(c.doc)
	if real == nil {
		return false
	}
	es := real.Scene()
	for _, n := range nodes {
		if eh := es.Handle(n); eh.IsZero() || !es.IsAlive(eh) {
			return false
		}
		ph := c.scene.Handle(n)
		if ph.IsZero() {
			return false
		}
		if _, ok := c.scene.GetByType(ph, t); !ok {
			return false
		}
	}
	return true
}

func (c *Controller) CopyToEdit(nodes []ids.NodeID, t schema.TypeID) error {
	if !c.CanCopyToEdit(nodes, t) {
		return errs.New(errs.EditorCommand, "these nodes do not exist in the edit scene")
	}
	real := c.ctx.Docs.Get(c.doc)
	es := real.Scene()
	td, ok := es.Registry().ByID(t)
	if !ok {
		return errs.New(errs.SchemaUnknownType, "component type is not registered")
	}
	tx := real.History.Begin("Copy " + td.FullName + " from Play")
	for _, n := range nodes {
		if _, has := es.GetByType(es.Handle(n), t); !has {
			values := make([]commands.FieldValue, 0, len(td.Fields))
			for i := range td.Fields {
				v, err := c.proxy.Props.Get(n, t, td.Fields[i].ID)
				if err != nil {
					tx.Abort()
					return err
				}
				values = append(values, commands.FieldValue{Field: td.Fields[i].ID, Value: v})
			}
			if err := tx.Exec(&commands.AddComponent{Node: n, Type: t, Name: td.FullName, Values: values}); err != nil {
				tx.Abort()
				return err
			}
			continue
		}
		for i := range td.Fields {
			fd := &td.Fields[i]
			if fd.Flags.Has(schema.FlagReadOnly) {
				continue
			}
			v, err := c.proxy.Props.Get(n, t, fd.ID)
			if err != nil {
				tx.Abort()
				return err
			}
			if err := real.Props.Set(tx, []ids.NodeID{n}, t, fd.ID, v); err != nil {
				tx.Abort()
				return err
			}
		}
	}
	tx.Commit()
	c.ctx.Notify(context.LevelInfo, "Copied "+td.FullName+" to the edit scene")
	return nil
}

func (c *Controller) SetCapture(on bool, origin mathx.Vec2, scale float32) {
	if c.state == Stopped {
		on = false
	}
	c.captured = on
	c.origin = origin
	if scale > 0 {
		c.scale = scale
	}
	if !on {
		c.input = input.State{}
	}
}

func (c *Controller) Captured() bool {
	return c.captured
}

func (c *Controller) syncInput() {
	if !c.captured {
		c.input = input.State{}
		return
	}
	f := c.ctx.Engine.Input().Snapshot()
	f.MousePos = mathx.Vec2{X: (f.MousePos.X - c.origin.X) * c.scale, Y: (f.MousePos.Y - c.origin.Y) * c.scale}
	f.MouseDelta = f.MouseDelta.Scale(c.scale)
	c.input.Restore(f)
}

func (c *Controller) guard(stage string) {
	if rec := recover(); rec != nil {
		c.ctx.Notify(context.LevelError, fmt.Sprintf("play mode stopped, %s panicked: %v", stage, rec))
		c.ctx.Log.Error("gameplay panic", log.F("stage", stage), log.F("panic", fmt.Sprint(rec)))
		c.Stop()
	}
}

func (c *Controller) Tick(dt float64) {
	if c.state == Stopped {
		return
	}
	if c.doc != 0 && c.ctx.Docs.Get(c.doc) == nil {
		c.Stop()
		return
	}
	c.syncInput()
	fixed := c.stepper.FixedDelta
	steps, alpha := 0, 0.0
	switch {
	case c.stepOnce:
		steps, dt = 1, fixed
		c.stepOnce = false
	case c.state == Paused:
		c.stats.Update = 0
		return
	default:
		steps, alpha = c.stepper.Advance(dt)
	}
	defer c.guard("update")
	start := time.Now()
	e := c.ctx.Engine
	for i := 0; i < steps; i++ {
		e.RunFixed(c.world, engine.ScopePlay, fixed)
	}
	e.RunVariable(c.world, engine.ScopePlay, dt, alpha)
	c.stats.Update = time.Since(start).Seconds()
	if c.scene != nil {
		c.stats.Nodes = c.scene.Stats().Alive - 1
	}
	c.frames++
	c.simTime += dt
}

func (c *Controller) Render(t *rt.Target) (cameras bool) {
	if c.state == Stopped || c.runner == nil || t == nil {
		return false
	}
	defer c.guard("render")
	start := time.Now()
	c.runner.SetTarget(t.Texture, t.BucketW, t.BucketH)
	c.runner.SetDepthTarget(t.Depth)
	c.runner.SetViewportSize(t.W, t.H)
	c.runner.ClearFixedView()
	e := c.ctx.Engine
	e.RunPhase(c.world, engine.PhaseRenderPrepare, engine.ScopePlay, 0, 0)
	e.RunPhase(c.world, engine.PhaseRender, engine.ScopePlay, 0, 0)
	rs := c.runner.Stats()
	c.stats.Render = time.Since(start).Seconds()
	c.stats.Sprites = rs.Sprites + rs.Sprites3D + rs.Meshes
	c.stats.Views = rs.Views
	return rs.Views > 0
}
