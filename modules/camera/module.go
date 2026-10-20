package camera

import (
	"aego/contracts/camera2d"
	"aego/core/clock"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/settings"
)

const (
	ID           = "camera"
	ServiceID    = ID + ".service"
	SystemUpdate = ID + ".update"
	ScriptModule = "aego/camera"
	needsScripts = "script"
)

type access struct {
	store     *settings.Store
	shakeID   string
	reducedID string
	shake     settings.Key
	reduced   settings.Key
}

func (a *access) read() (float32, bool) {
	if a == nil || a.store == nil {
		return 1, false
	}
	if a.shake == 0 && a.shakeID != "" {
		a.shake = a.store.Key(a.shakeID)
	}
	if a.reduced == 0 && a.reducedID != "" {
		a.reduced = a.store.Key(a.reducedID)
	}
	scale := float32(1)
	if a.shake != 0 {
		scale = float32(a.store.Float(a.shake))
	}
	return scale, a.reduced != 0 && a.store.Bool(a.reduced)
}

type Module struct {
	acc access
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Requires: []string{needsScripts}, Builtin: true,
		DisplayName: "Cameras 2D", DocsOrder: 45, DocsIcon: "comp.vcam"}
}

func (*Module) IconDir() string {
	return "modules/camera/res/icons/src"
}

func (m *Module) Register(r *module.Registry) error {
	if err := registerTypes(r); err != nil {
		return err
	}
	r.System(engine.SystemDesc{ID: SystemUpdate, Phase: engine.PhasePostUpdate, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: update{}})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule))
	}
	registerDocs(r)
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.acc = access{store: ctx.Settings}
	for _, p := range ctx.Provided(camera2d.AccessibilityKind) {
		if a, ok := p.Value.(camera2d.Accessibility); ok {
			m.acc.shakeID, m.acc.reducedID = a.ScreenShake, a.ReducedMotion
		}
	}
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	s := NewService(ctx.Scene, ctx.World)
	s.acc = &m.acc
	ctx.World.SetService(ServiceID, s)
	ctx.World.SetView(camera2d.ServiceID, camera2d.Adjuster(s))
	if s.hub != nil {
		s.hub.Probe(typeBlend, s.blendAlive)
	}
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	if s := Of(ctx.World); s != nil && s.hub != nil {
		s.hub.Probe(typeBlend, nil)
	}
	ctx.World.RemoveService(camera2d.ServiceID)
	ctx.World.RemoveService(ServiceID)
}

func Of(w *engine.World) *Service {
	if w == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

type update struct{}

func (update) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	if s == nil {
		return
	}
	game, real := float32(ctx.Dt), float32(ctx.Unscaled)
	if t := ctx.Time; t != nil {
		game = float32(t.Span(clock.Game).Delta)
		real = float32(t.Span(clock.Real).Delta)
	}
	s.Update(game, real)
}
