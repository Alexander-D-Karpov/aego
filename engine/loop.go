package engine

import (
	"runtime/metrics"
	"time"

	"aego/core/clock"
	"aego/core/errs"
	"aego/platform"
)

const idleSleep = 16 * time.Millisecond

func (e *Engine) Step(app App) bool {
	if e.closed || e.err != nil {
		return false
	}
	if !e.appInit {
		e.app = app
		if err := app.Init(e); err != nil {
			e.err = errs.Wrap(err, errs.EngineInit, "app initialization failed")
			return false
		}
		e.appInit = true
		e.lastTime = e.plat.Now()
	}
	e.frame()
	return !e.quit
}

func gated(p Phase) bool {
	return p >= PhaseFixedBegin && p <= PhaseAnimation
}

func (e *Engine) exec(ctx *Context, p Phase, mask SystemScope, paused bool) {
	list := e.reg.systems.phases[p]
	if len(list) == 0 {
		return
	}
	var before uint64
	if e.probe {
		before = e.objects()
	}
	gate := gated(p)
	ps := e.tracer.Begin(e.phaseZones[p])
	for i := range list {
		s := &list[i]
		if s.scope&mask == 0 || (gate && !s.pause.runs(paused)) {
			continue
		}
		z := e.tracer.Begin(s.zone)
		s.sys.Run(ctx)
		z.End()
	}
	ps.End()
	if e.probe {
		e.allocs[p] += e.objects() - before
	}
}
func (e *Engine) SetPhaseAllocs(on bool) {
	if on && len(e.probeSample) == 0 {
		e.probeSample = []metrics.Sample{{Name: "/gc/heap/allocs:objects"}}
	}
	e.probe = on
}
func (e *Engine) objects() uint64 {
	metrics.Read(e.probeSample)
	if v := e.probeSample[0].Value; v.Kind() == metrics.KindUint64 {
		return v.Uint64()
	}
	return 0
}
func (e *Engine) PhaseAllocs(dst []uint64) []uint64 {
	return append(dst, e.lastAllocs[:]...)
}

func (e *Engine) begin(ctx *Context, w *World, real float64) clock.Plan {
	t := w.Time
	plan := t.Begin(real)
	rnd := e.ctx.Random
	*ctx = Context{
		Engine:   e,
		World:    w,
		Time:     t,
		Dt:       t.Span(clock.Game).Delta,
		Unscaled: t.Span(clock.Real).Delta,
		Alpha:    t.Alpha(),
		Frame:    t.Frame(),
		Tick:     t.Tick(),
		Paused:   plan.Paused,
		Random:   rnd,
	}
	if w.Recorder != nil {
		w.Recorder.Stamp(uint64(w.ID()), ctx.Frame, ctx.Tick)
	}
	return plan
}

func (e *Engine) mark(w *World, steps int) {
	t := w.Time
	var first, last uint64
	if steps > 0 {
		first, last = t.Tick()+1, t.Tick()+uint64(steps)
	}
	e.tracer.Mark(uint64(w.ID()), t.Frame(), first, last)
}

func (e *Engine) simulate(ctx *Context, mask SystemScope, plan clock.Plan) {
	t, w := ctx.Time, ctx.World
	fixed := t.FixedDelta()
	paused := plan.Paused
	for range plan.Steps {
		ctx.Tick = t.BeginTick()
		if w.Recorder != nil {
			w.Recorder.Stamp(uint64(w.ID()), ctx.Frame, ctx.Tick)
		}
		ctx.Dt, ctx.Alpha = fixed, 0
		for p := PhaseFixedBegin; p <= PhaseFixedEnd; p++ {
			e.exec(ctx, p, mask, false)
		}
		if w.Break != nil && w.Break() {
			t.SetPaused(true)
			paused = true
			break
		}
	}
	ctx.Paused = paused
	ctx.Dt, ctx.Alpha = t.Span(clock.Game).Delta, t.Alpha()
	if paused {
		ctx.Dt = 0
	}
	for p := PhasePreUpdate; p <= PhaseAnimation; p++ {
		e.exec(ctx, p, mask, paused)
	}
	if !paused && w.Break != nil && w.Break() {
		t.SetPaused(true)
	}
}

func (e *Engine) TickWorld(w *World, scope SystemScope, real float64) clock.Plan {
	if w == nil || w.Time == nil {
		return clock.Plan{}
	}
	saved := e.wctx
	plan := e.begin(&e.wctx, w, real)
	e.mark(w, plan.Steps)
	e.exec(&e.wctx, PhaseFrameBegin, scope, plan.Paused)
	e.exec(&e.wctx, PhaseInput, scope, plan.Paused)
	e.simulate(&e.wctx, scope, plan)
	e.wctx = saved
	return plan
}

func (e *Engine) RunPhase(w *World, p Phase, scope SystemScope, dt, alpha float64) {
	if w == nil || int(p) >= phaseCount {
		return
	}
	saved := e.wctx
	e.wctx = Context{Engine: e, World: w, Dt: dt, Alpha: alpha, Frame: e.frameNo, Random: e.ctx.Random}
	if t := w.Time; t != nil {
		e.wctx.Time, e.wctx.Frame, e.wctx.Tick = t, t.Frame(), t.Tick()
		e.wctx.Unscaled = t.Span(clock.Real).Delta
	}
	e.exec(&e.wctx, p, scope, false)
	e.wctx = saved
}

func (e *Engine) RunFixed(w *World, scope SystemScope, dt float64) {
	for p := PhaseFixedBegin; p <= PhaseFixedEnd; p++ {
		e.RunPhase(w, p, scope, dt, 0)
	}
}

func (e *Engine) RunVariable(w *World, scope SystemScope, dt, alpha float64) {
	for p := PhasePreUpdate; p <= PhaseAnimation; p++ {
		e.RunPhase(w, p, scope, dt, alpha)
	}
}

func (e *Engine) frame() {
	e.lastAllocs, e.allocs = e.allocs, [phaseCount]uint64{}
	e.tracer.BeginFrame()
	now := e.plat.Now()
	delta := 0.0
	if e.frameNo > 0 {
		delta = now - e.lastTime
	}
	if delta < 0 {
		delta = 0
	}
	e.lastTime = now
	e.frameNo++
	mask := ScopeFrame | e.cfg.LoopScope
	plan := e.begin(&e.ctx, e.main, delta)
	e.mark(e.main, plan.Steps)

	e.jobs.drain()
	e.exec(&e.ctx, PhaseFrameBegin, mask, plan.Paused)

	e.input.BeginFrame()
	pz := e.tracer.Begin(e.zPoll)
	e.plat.PollEvents(e.onEvent)
	pz.End()
	e.exec(&e.ctx, PhasePollEvents, mask, plan.Paused)
	e.exec(&e.ctx, PhaseInput, mask, plan.Paused)

	updateStart := e.plat.Now()
	e.simulate(&e.ctx, mask, plan)
	renderStart := e.plat.Now()
	e.stats.UpdateTime = renderStart - updateStart

	e.stats.ResetGPU()
	if !e.minimized && e.metrics.PixelW > 0 && e.metrics.PixelH > 0 {
		e.exec(&e.ctx, PhaseRenderPrepare, mask, plan.Paused)
		e.curFrame = e.dev.BeginFrame(e.metrics.PixelW, e.metrics.PixelH)
		e.exec(&e.ctx, PhaseRender, mask, plan.Paused)
		e.exec(&e.ctx, PhaseRenderUI, mask, plan.Paused)
		gs := e.curFrame.Stats()
		e.dev.EndFrame()
		e.curFrame = nil
		sz := e.tracer.Begin(e.zSwap)
		e.win.SwapBuffers()
		sz.End()
		e.stats.DrawCalls = gs.DrawCalls
		e.stats.Vertices = gs.Vertices
		e.stats.Triangles = gs.Triangles
		e.stats.TexSwitches = gs.TexSwitches
	} else if e.minimized {
		time.Sleep(idleSleep)
	}
	e.stats.RenderTime = e.plat.Now() - renderStart

	if lim := e.frameLimit; lim > 0 && e.vsync == platform.VSyncOff && !e.cfg.Headless {
		if wait := 1/lim - (e.plat.Now() - now); wait > 0 {
			time.Sleep(time.Duration(wait * float64(time.Second)))
		}
	}
	e.exec(&e.ctx, PhaseFrameEnd, mask, plan.Paused)
	e.stats.FrameTime = delta
	e.counter.Sample(&e.stats)
	e.tracer.EndFrame()
}
