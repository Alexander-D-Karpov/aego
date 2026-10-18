package play

import (
	"fmt"
	"runtime/debug"
	"strconv"
	"time"

	"aego/asset"
	"aego/contracts/persistence"
	"aego/core/clock"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/core/rng"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history"
	"aego/editor/core/history/commands"
	"aego/engine"
	"aego/engine/recorder"
	"aego/platform/input"
	"aego/render/rt"
	"aego/render/view"
	"aego/scene"
	"aego/scene/frame"
	"aego/scene/sceneio"
	"aego/schema"
)

type State uint8

const (
	Stopped State = iota
	Playing
	Paused
)

type Stats struct {
	Update       float64
	Render       float64
	Nodes        int
	Sprites      int
	Views        int
	Frame        uint64
	Updates      uint64
	Tick         uint64
	Alpha        float64
	Scale        float64
	DroppedFixed uint64
}

type Controller struct {
	ctx       *context.Editor
	state     State
	world     *engine.World
	runner    *frame.Runner
	scene     *scene.Scene
	doc       document.ID
	scale     float64
	input     input.State
	captured  bool
	origin    mathx.Vec2
	size      mathx.Vec2
	stats     Stats
	proxy     *document.SceneDocument
	subs      []stateSub
	nextSub   uint64
	detach    func()
	unrec     func()
	breakFn   func() (string, bool)
	breakWhy  string
	release   func(*input.State) bool
	hint      func() string
	hist      *recording
	limit     int
	cursor    int
	beyond    bool
	replaying bool
	stepReq   uint8
	seed      uint64
	blocked   string
	assetRev  uint64
	editUnsub func()
	breaks    int32
	broke     int32
}

func New(ctx *context.Editor) *Controller {
	return &Controller{ctx: ctx, scale: 1}
}

func (c *Controller) SetRelease(pressed func(*input.State) bool, hint func() string) {
	c.release, c.hint = pressed, hint
}

func (c *Controller) ReleasePressed(in *input.State) bool {
	return c.release != nil && c.release(in)
}

func (c *Controller) ReleaseHint() string {
	if c.hint == nil {
		return ""
	}
	if k := c.hint(); k != "" {
		return k + " releases input"
	}
	return ""
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

func (c *Controller) Views() []view.View {
	if c.state == Stopped || c.runner == nil {
		return nil
	}
	return c.runner.Views()
}

func (c *Controller) World() *engine.World {
	return c.world
}

func (c *Controller) Doc() document.ID {
	return c.doc
}

func (c *Controller) Frames() uint64 {
	if c.world == nil {
		return 0
	}
	return c.world.Time.Frame()
}
func (c *Controller) Ticks() uint64 {
	if c.world == nil {
		return 0
	}
	return c.world.Time.Tick()
}

func (c *Controller) SimTime() float64 {
	if c.world == nil {
		return 0
	}
	return c.world.Time.Span(clock.Game).Elapsed
}
func (c *Controller) TimeScale() float64 {
	return c.scale
}
func (c *Controller) SetTimeScale(s float64) {
	c.scale = min(max(s, 0), clock.MaxScale)
	if c.world != nil {
		c.world.Time.SetScale(c.scale)
	}
}

func (c *Controller) SetBreak(fn func() (string, bool)) {
	c.breakFn = fn
}

func (c *Controller) Recorder() *recorder.Recorder {
	if c.world == nil {
		return nil
	}
	return c.world.Recorder
}

func (c *Controller) checkBreak() bool {
	if c.breakFn == nil {
		return false
	}
	c.breaks++
	why, hit := c.breakFn()
	if hit {
		c.breakWhy = why
		c.broke = c.breaks
	}
	return hit
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
	if first, n := c.ctx.Problems.Blocking(); n > 0 {
		return errs.New(errs.EditorCommand, strconv.Itoa(n)+" problem(s) block Play, first: "+first.Message)
	}
	if c.state != Stopped {
		c.Stop()
	}
	real := c.ctx.Docs.Get(d.ID)
	if real == nil {
		return errs.New(errs.EditorDocument, "scene is not open")
	}
	data := sceneio.Save(real.Scene())
	c.seed = uint64(time.Now().UnixNano()) | 1
	if err := c.start(data, real.Path); err != nil {
		return err
	}
	c.hist = newHistory(data, real.Path, c.limit)
	c.cursor, c.beyond, c.stepReq = 0, false, stepNone
	c.assetRev = c.ctx.AssetRev
	c.doc = real.ID
	c.proxy = real.PlayCopy(c.scene)
	c.ctx.Docs.SetPlay(c.proxy, c.world)
	c.watchEdits()
	c.notify()
	c.ctx.Notify(context.LevelInfo, "Play mode started")
	return nil
}

func (c *Controller) SceneID() ids.AssetID {
	real := c.ctx.Docs.Get(c.doc)
	if real == nil || c.ctx.Pipeline == nil {
		return ids.AssetID{}
	}
	id, _ := c.ctx.Pipeline.Lookup(real.Path)
	return id
}

func (c *Controller) LoadScene(id ids.AssetID) error {
	ctx := c.ctx
	if ctx.Pipeline == nil {
		return errs.New(errs.EditorDocument, "the asset pipeline is not available")
	}
	rel, ok := ctx.Pipeline.PathOf(id)
	if !ok {
		return errs.New(errs.EditorDocument, "the saved scene is not in the project")
	}
	d, err := ctx.Docs.Open(rel)
	if err != nil {
		return err
	}
	return c.Start(d)
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
	c.world.Random = rng.New(c.seed)
	c.world.Time.Configure(ctx.Engine.TimeConfig())
	c.world.Time.SetScale(c.scale)
	c.world.Recorder = recorder.New(4096)
	c.world.Break = c.checkBreak
	c.runner = frame.New()
	c.runner.Bind(s, ctx.Renderer, res)
	c.world.SetService(frame.ServiceID, c.runner)
	c.world.SetService(scene.ServiceID, s)
	c.world.SetService(persistence.HostID, c)
	c.unrec = frame.Record(s, c.world.Recorder)
	c.world.Recorder.Add(recorder.SceneChange, ids.NodeID{}, ids.NodeID{}, name, "play")
	c.input = input.State{}
	if ctx.Scripts != nil {
		detach, err := ctx.Scripts.Attach(c.world, s)
		if err != nil {
			c.world, c.runner, c.scene = nil, nil, nil
			return errs.Wrap(err, errs.EditorDocument, "cannot start the scripts")
		}
		c.detach = detach
	}
	c.state = Playing
	return nil
}

func (c *Controller) teardown() {
	if c.editUnsub != nil {
		c.editUnsub()
		c.editUnsub = nil
	}
	if c.unrec != nil {
		c.unrec()
		c.unrec = nil
	}
	if c.detach != nil {
		c.detach()
		c.detach = nil
	}
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
	c.world.Time.SetPaused(c.state == Paused)
	c.notify()
}

func (c *Controller) Step() {
	if c.state == Paused {
		c.world.Time.StepFrame()
		c.stepReq = stepFrame
	}
}

func (c *Controller) StepFixed() {
	if c.state == Paused {
		c.world.Time.StepFixed()
		c.stepReq = stepFixed
	}
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
	c.teardown()
	c.ctx.Docs.ClearPlay()
	c.proxy = nil
	c.state = Stopped
	c.world, c.runner, c.scene = nil, nil, nil
	c.doc = 0
	c.captured = false
	c.input = input.State{}
	c.stats = Stats{}
	c.hist, c.cursor, c.beyond, c.stepReq = nil, 0, false, stepNone
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

func (c *Controller) SetCapture(on bool, origin, size mathx.Vec2) {
	if c.state == Stopped {
		on = false
	}
	c.captured = on
	c.origin, c.size = origin, size
	if !on {
		c.input = input.State{}
	}
}

func (c *Controller) Captured() bool {
	return c.captured
}

func (c *Controller) syncInput() {
	in := c.ctx.Engine.Input()
	if !c.captured {
		c.input = input.State{}
		c.input.CopyPads(in)
		return
	}
	f := in.Snapshot()
	pos := f.MousePos.Sub(c.origin)
	outside := pos.X < 0 || pos.Y < 0 || pos.X >= c.size.X || pos.Y >= c.size.Y
	click := in.ButtonPressed(input.MouseLeft) || in.ButtonPressed(input.MouseRight) || in.ButtonPressed(input.MouseMiddle)
	if c.ReleasePressed(in) || (outside && click) {
		c.captured = false
		c.input = input.State{}
		c.input.CopyPads(in)
		return
	}
	f.MousePos = pos
	c.input.Restore(f)
	c.input.CopyPads(in)
}

func (c *Controller) guard(stage string) {
	if rec := recover(); rec != nil {
		if c.ctx.Crash != nil {
			c.ctx.Crash("play "+stage, rec, debug.Stack())
		}
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
	defer c.guard("update")
	c.syncInput()
	h := c.hist
	if h != nil && c.cursor < len(h.frames) && c.state == Paused && c.stepReq == stepNone {
		return
	}
	w := c.world
	recorded := h != nil && c.record(dt)
	c.breaks, c.broke = 0, 0
	start := time.Now()
	plan := c.ctx.Engine.TickWorld(w, engine.ScopePlay, dt)
	if c.state == Stopped {
		return
	}
	if recorded {
		f := &h.frames[len(h.frames)-1]
		f.tick, f.brk = w.Time.Tick(), c.broke
	}
	c.stats.Update = 0
	if plan.Update || plan.Steps > 0 {
		c.stats.Update = time.Since(start).Seconds()
	}
	t := w.Time
	c.stats.Frame, c.stats.Updates, c.stats.Tick, c.stats.Alpha = t.Frame(), t.Updates(), t.Tick(), t.Alpha()
	c.stats.Scale, c.stats.DroppedFixed = t.Scale(), t.DroppedSteps()
	if c.scene != nil {
		c.stats.Nodes = c.scene.Stats().Alive - 1
	}
	if paused := t.Paused(); paused != (c.state == Paused) {
		c.state = Playing
		if paused {
			c.state = Paused
		}
		c.notify()
	}
	if c.breakWhy != "" {
		c.ctx.Notify(context.LevelWarn, "Paused on tick "+strconv.FormatUint(t.Tick(), 10)+": "+c.breakWhy)
		c.breakWhy = ""
	}
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
	alpha := c.world.Time.Alpha()
	e.RunPhase(c.world, engine.PhaseRenderPrepare, engine.ScopePlay, 0, alpha)
	e.RunPhase(c.world, engine.PhaseRender, engine.ScopePlay, 0, alpha)
	e.RunPhase(c.world, engine.PhaseRenderUI, engine.ScopePlay, 0, alpha)
	rs := c.runner.Stats()
	c.stats.Render = time.Since(start).Seconds()
	c.stats.Sprites = rs.Sprites + rs.Sprites3D + rs.Meshes
	c.stats.Views = rs.Views
	return rs.Views > 0
}
