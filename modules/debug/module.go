package debug

import (
	"aego/contracts/debugdraw"
	"aego/core/mathx"
	"aego/engine"
	"aego/engine/module"
	"aego/scene/frame"
)

const (
	ID           = "debug"
	SystemExpire = ID + ".expire"
	SystemDraw   = ID + ".draw"
	ScriptModule = "aego/debug"
)

type Module struct {
	settings Settings
}

func New() *Module {
	return &Module{settings: DefaultSettings()}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Debug draw", DocsOrder: 100}
}

func (m *Module) Register(r *module.Registry) error {
	if err := registerSettings(r); err != nil {
		return err
	}
	r.System(engine.SystemDesc{ID: SystemExpire, Phase: engine.PhaseFrameBegin, Scope: engine.ScopePlay, System: expireSystem{}})
	r.System(engine.SystemDesc{ID: SystemDraw, Phase: engine.PhaseRender, Scope: engine.ScopePlay, After: []string{frame.SystemSubmit}, System: drawSystem{}})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule))
	}
	registerDocs(r)
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	if s, ok := module.SectionValue[Settings](ctx.Sections, ID); ok {
		m.settings = *s
	}
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil {
		return nil
	}
	ctx.World.SetService(debugdraw.ServiceID, NewService(m.settings))
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if ctx.Play && ctx.World != nil {
		ctx.World.RemoveService(debugdraw.ServiceID)
	}
}

type expireSystem struct{}

func (expireSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Advance(ctx.Paused, ctx.Dt, ctx.Unscaled)
	}
}

type drawSystem struct{}

func (drawSystem) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	if s == nil || !s.cfg.Enabled || len(s.prims) == 0 {
		return
	}
	r := frame.Of(ctx)
	if r == nil || r.Renderer() == nil {
		return
	}
	rnd := r.Renderer()
	font := rnd.UIFont()
	for _, v := range r.Views() {
		if v.Is3D() {
			continue
		}
		v.DoClear = false
		v.Label = SystemDraw
		f := rnd.View(v)
		fv := f.View()
		px := fv.ScreenToWorld(mathx.Vec2{X: 1}).Sub(fv.ScreenToWorld(mathx.Vec2{})).Len()
		s.Lines(f.Debug(), px, fv.PixelScale, true)
		s.Texts(f, font, px)
		f.End()
	}
}
