package gameplay

import (
	"errors"

	"aego/core/clock"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
)

const (
	ID              = "gameplay"
	ServiceID       = ID + ".service"
	SystemMotion    = ID + ".motion"
	SystemPost      = ID + ".post"
	SystemFrame     = ID + ".frame"
	SystemFSMFixed  = ID + ".fsm_fixed"
	SystemFSMUpdate = ID + ".fsm_update"
	scriptUpdate    = "script.update"
	Category        = "Gameplay"
	needsPhysics    = "physics"
	needsScripts    = "script"
	scriptFixed     = "script.fixed"
	scriptEvents    = "script.events_fixed"

	channel log.Channel = "gameplay"
)

type Module struct {
	sig signals
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{needsPhysics, needsScripts}, Builtin: true,
		DisplayName: "Gameplay", DocsOrder: 50, DocsIcon: "cat.gameplay"}
}

func (*Module) IconDir() string {
	return "modules/gameplay/res/icons/src"
}

func (m *Module) Register(r *module.Registry) error {
	if err := errors.Join(registerMotion(r, &m.sig), registerCombat(r, &m.sig), registerZones(r, &m.sig), registerFSM(r, &m.sig)); err != nil {
		return err
	}
	registerTemplates(r)
	r.SaveSection(ID+".respawn", respawnSection{})
	r.System(engine.SystemDesc{ID: SystemFSMFixed, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, After: []string{scriptFixed}, Before: []string{SystemMotion}, System: fsmFixedSystem{}})
	r.System(engine.SystemDesc{ID: SystemMotion, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, After: []string{scriptFixed}, System: fixedSystem{}})
	r.System(engine.SystemDesc{ID: SystemPost, Phase: engine.PhasePostPhysics, Scope: engine.ScopePlay, Before: []string{scriptEvents}, System: postSystem{}})
	r.System(engine.SystemDesc{ID: SystemFSMUpdate, Phase: engine.PhaseUpdate, Scope: engine.ScopePlay, After: []string{scriptUpdate}, System: fsmUpdateSystem{}})
	r.System(engine.SystemDesc{ID: SystemFrame, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: frameSystem{}})
	registerDocs(r)
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	s := NewService(ctx.Scene, ctx.World, nil)
	s.sig = &m.sig
	if ctx.Log != nil {
		s.log = ctx.Log.With(channel)
	}
	ctx.World.SetService(ServiceID, s)
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if ctx.Play && ctx.World != nil {
		ctx.World.RemoveService(ServiceID)
	}
}

func Of(w *engine.World) *Service {
	if w == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

type fixedSystem struct{}

func (fixedSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Fixed(float32(ctx.Dt))
	}
}

type postSystem struct{}

func (postSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Post()
	}
}

type frameSystem struct{}

func (frameSystem) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	if s == nil {
		return
	}
	game, real := float32(ctx.Dt), float32(ctx.Unscaled)
	if t := ctx.Time; t != nil {
		game = float32(t.Span(clock.Game).Delta)
		real = float32(t.Span(clock.Real).Delta)
	}
	s.Frame(game, real)
}

type fsmFixedSystem struct{}

func (fsmFixedSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.FSMFixed(float32(ctx.Dt))
	}
}

type fsmUpdateSystem struct{}

func (fsmUpdateSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.FSMUpdate(float32(ctx.Dt))
	}
}
