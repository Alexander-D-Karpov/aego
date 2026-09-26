package engine

import (
	"time"

	"aego/core/errs"
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

func (e *Engine) runPhase(p Phase) {
	mask := ScopeFrame | e.cfg.LoopScope
	for _, s := range e.reg.systems.phases[p] {
		if s.scope&mask != 0 {
			s.sys.Run(&e.ctx)
		}
	}
}

func (e *Engine) RunPhase(w *World, p Phase, scope SystemScope, dt, alpha float64) {
	if w == nil || int(p) >= phaseCount {
		return
	}
	saved := e.wctx
	e.wctx = Context{Engine: e, World: w, Dt: dt, Alpha: alpha, Frame: e.frameNo, Random: e.ctx.Random}
	for _, s := range e.reg.systems.phases[p] {
		if s.scope&scope != 0 {
			s.sys.Run(&e.wctx)
		}
	}
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
	e.ctx.Frame = e.frameNo
	e.ctx.Dt = delta
	e.ctx.Alpha = 0

	e.jobs.drain()
	e.runPhase(PhaseFrameBegin)

	e.input.BeginFrame()
	e.plat.PollEvents(e.onEvent)
	e.runPhase(PhasePollEvents)
	e.runPhase(PhaseInput)

	updateStart := e.plat.Now()
	steps, alpha := e.stepper.Advance(delta)
	fixed := e.stepper.FixedDelta
	for i := 0; i < steps; i++ {
		e.ctx.Dt = fixed
		e.ctx.Alpha = 0
		for p := PhaseFixedBegin; p <= PhaseFixedEnd; p++ {
			e.runPhase(p)
		}
	}
	e.ctx.Dt = delta
	e.ctx.Alpha = alpha
	for p := PhasePreUpdate; p <= PhaseAnimation; p++ {
		e.runPhase(p)
	}
	renderStart := e.plat.Now()
	e.stats.UpdateTime = renderStart - updateStart

	e.stats.ResetGPU()
	if !e.minimized && e.metrics.PixelW > 0 && e.metrics.PixelH > 0 {
		e.runPhase(PhaseRenderPrepare)
		e.curFrame = e.dev.BeginFrame(e.metrics.PixelW, e.metrics.PixelH)
		e.runPhase(PhaseRender)
		e.runPhase(PhaseRenderUI)
		gs := e.curFrame.Stats()
		e.dev.EndFrame()
		e.curFrame = nil
		e.win.SwapBuffers()
		e.stats.DrawCalls = gs.DrawCalls
		e.stats.Vertices = gs.Vertices
		e.stats.Triangles = gs.Triangles
		e.stats.TexSwitches = gs.TexSwitches
	} else if e.minimized {
		time.Sleep(idleSleep)
	}
	e.stats.RenderTime = e.plat.Now() - renderStart

	e.runPhase(PhaseFrameEnd)
	e.stats.FrameTime = delta
	e.counter.Sample(&e.stats)
}
