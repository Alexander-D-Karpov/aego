package gameplay

import (
	"aego/contracts/physics2d"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
)

const (
	ID               = "gameplay"
	ServiceID        = ID + ".service"
	SystemMotion     = ID + ".motion"
	SystemMotionPost = ID + ".motion_post"
	Category         = "Gameplay"
	needsPhysics     = "physics"
	needsScripts     = "script"
	scriptFixed      = "script.fixed"
	scriptEvents     = "script.events_fixed"

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
	if err := registerMotion(r, &m.sig); err != nil {
		return err
	}
	r.System(engine.SystemDesc{ID: SystemMotion, Phase: engine.PhasePrePhysics, Scope: engine.ScopePlay, After: []string{scriptFixed}, System: fixedSystem{}})
	r.System(engine.SystemDesc{ID: SystemMotionPost, Phase: engine.PhasePostPhysics, Scope: engine.ScopePlay, Before: []string{scriptEvents}, System: postSystem{}})
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

// queries finds the physics queries lazily: physics may attach to the world after this module.
func (s *Service) queries() physics2d.Queries {
	if s.q == nil && s.world != nil {
		s.q, _ = engine.ServiceOf[physics2d.Queries](s.world, physics2d.ServiceID)
	}
	return s.q
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
