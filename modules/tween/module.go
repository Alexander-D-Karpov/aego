package tween

import (
	"aego/engine"
	"aego/engine/module"
	"aego/script/abi"
)

const (
	ID             = "tween"
	ServiceID      = ID + ".service"
	SystemGame     = ID + ".game"
	SystemUnscaled = ID + ".unscaled"
	ScriptTween    = "aego/tween"
	ScriptTimer    = "aego/timer"
	KindTween      = ScriptTween + ".Tween"
	KindTimer      = ScriptTimer + ".Timer"
	needsScripts   = "script"
)

var (
	typeTween = abi.HandleID(KindTween)
	typeTimer = abi.HandleID(KindTimer)
)

type Module struct{}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{needsScripts}, Builtin: true}
}

func (*Module) Register(r *module.Registry) error {
	registerDocs(r)
	r.System(engine.SystemDesc{ID: SystemGame, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, System: step{}})
	r.System(engine.SystemDesc{
		ID: SystemUnscaled, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, Pause: engine.PauseAlways,
		After: []string{SystemGame}, System: step{unscaled: true},
	})
	if r.Bindings() != nil {
		installTween(r.ScriptModule(ScriptTween))
		installTimer(r.ScriptModule(ScriptTimer))
	}
	return nil
}

func (*Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	ctx.World.SetService(ServiceID, NewService(ctx.Scene, ctx.World))
	return nil
}

func (*Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	if s := Of(ctx.World); s != nil {
		s.Close()
	}
	ctx.World.RemoveService(ServiceID)
}

func Of(w *engine.World) *Service {
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

type step struct {
	unscaled bool
}

func (st step) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	if s == nil {
		return
	}
	dt := ctx.Dt
	if st.unscaled {
		dt = ctx.Unscaled
	}
	s.Step(float32(dt), st.unscaled)
}
