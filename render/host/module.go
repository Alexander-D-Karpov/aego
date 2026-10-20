package host

import (
	"aego/core/log"
	"aego/engine"
	"aego/render"
)

const (
	ID          = "aego.render"
	ServiceID   = "render"
	SystemBegin = "render.begin"
	SystemEnd   = "render.end"
)

type Module struct {
	opt   render.Options
	r     *render.Renderer
	eng   *engine.Engine
	stats render.Stats
	scale float32
}

func New(opt render.Options) *Module {
	return &Module{opt: opt, scale: 1}
}

func (m *Module) Info() engine.ModuleInfo {
	return engine.ModuleInfo{ID: ID, Version: "0.2.0"}
}

func (m *Module) Register(reg *engine.Registry) error {
	sys := reg.Systems()
	if err := sys.Register(engine.SystemDesc{
		ID:     SystemBegin,
		Phase:  engine.PhaseRender,
		Scope:  engine.ScopeFrame,
		Before: []string{engine.SystemAppRender},
		System: beginSystem{m: m},
	}); err != nil {
		return err
	}
	return sys.Register(engine.SystemDesc{
		ID:     SystemEnd,
		Phase:  engine.PhaseRenderUI,
		Scope:  engine.ScopeFrame,
		System: endSystem{m: m},
	})
}

func (m *Module) Init(e *engine.Engine) error {
	r, err := render.New(e.GPU(), e.Caps(), e.VFS(), e.Log(), m.opt)
	if err != nil {
		return err
	}
	r.SetTracer(e.Trace())
	m.r = r
	m.eng = e
	return nil
}

func (m *Module) Renderer() *render.Renderer {
	return m.r
}

func (m *Module) Stats() render.Stats {
	return m.stats
}

func (m *Module) Close() {
	if m.r != nil {
		m.r.Close()
		m.r = nil
	}
}

type beginSystem struct {
	m *Module
}

func (s beginSystem) Run(ctx *engine.Context) {
	m := s.m
	if m.r == nil {
		return
	}
	metrics := ctx.Engine.Metrics()
	scale := metrics.DisplayScale
	if scale <= 0 {
		scale = 1
	}
	if err := m.r.BeginFrame(ctx.Engine.Frame(), metrics.PixelW, metrics.PixelH, scale); err != nil {
		ctx.Engine.Log().With(log.Render).Error("render begin frame failed", log.F("err", err.Error()))
	}
}

type endSystem struct {
	m *Module
}

func (s endSystem) Run(*engine.Context) {
	if s.m.r == nil {
		return
	}
	s.m.stats = s.m.r.EndFrame()
}
