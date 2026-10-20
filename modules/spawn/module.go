package spawn

import (
	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/engine/module"
)

const (
	ID             = "spawn"
	ServiceID      = ID + ".service"
	SystemPark     = ID + ".park"
	EventSpawned   = "OnSpawned"
	EventDespawned = "OnDespawned"
	scriptScene    = "aego/scene"
)

type Events struct {
	Spawned   scriptevents.EventID
	Despawned scriptevents.EventID
}

type Module struct {
	ev Events
}

func New() *Module {
	return &Module{ev: Events{Spawned: scriptevents.NoEvent, Despawned: scriptevents.NoEvent}}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Spawning", DocsOrder: 70, DocsIcon: "file.prefab"}
}

func (m *Module) Register(r *module.Registry) error {
	registerDocs(r)
	m.ev.Spawned = r.ScriptEvent(scriptevents.Event{Name: EventSpawned,
		Doc: "OnSpawned runs on every script of a spawned prefab copy, after Ready for a new copy and instead of it for a copy taken from a pool."})
	m.ev.Despawned = r.ScriptEvent(scriptevents.Event{Name: EventDespawned,
		Doc: "OnDespawned runs on every script of a pooled copy when it is despawned, before its coroutines and connections end."})
	r.System(engine.SystemDesc{ID: SystemPark, Phase: engine.PhaseAnimation, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: park{}})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(scriptScene))
	}
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	ctx.World.SetService(ServiceID, NewService(ctx.Scene, ctx.World, m.ev))
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
	if w == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

type park struct{}

func (park) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Park()
	}
}
