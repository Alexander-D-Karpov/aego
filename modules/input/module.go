package input

import (
	"aego/contracts/inputactions"
	"aego/core/clock"
	"aego/core/log"
	"aego/engine"
	"aego/engine/module"
)

const (
	ServiceID     = ID + ".service"
	SystemFrame   = ID + ".frame"
	SystemTick    = ID + ".tick"
	SystemTickEnd = ID + ".tick_end"
)

type Module struct {
	cfg   Config
	files Files
	keys  UserKeys
}

func New() *Module {
	return &Module{cfg: Default()}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true}
}

func (m *Module) Register(r *module.Registry) error {
	r.ProjectSection(Section())
	r.System(engine.SystemDesc{ID: SystemFrame, Phase: engine.PhaseInput, Scope: engine.ScopePlay, Pause: engine.PauseAlways, System: frameSystem{}})
	r.System(engine.SystemDesc{ID: SystemTick, Phase: engine.PhaseFixedBegin, Scope: engine.ScopePlay, System: tickSystem{}})
	r.System(engine.SystemDesc{ID: SystemTickEnd, Phase: engine.PhaseFixedEnd, Scope: engine.ScopePlay, System: tickEndSystem{}})
	if r.Bindings() != nil {
		installScript(r.ScriptModule(ScriptModule))
	}
	registerDocs(r)
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	cfg := Default()
	if c, ok := module.SectionValue[Config](ctx.Sections, ID); ok {
		cfg = *c
	}
	cfg = cfg.clone()
	if err := merge(&cfg, ctx.Provided(inputactions.Kind)); err != nil {
		return err
	}
	m.cfg = cfg
	if ctx.Engine != nil {
		m.files = ctx.Engine.VFS()
	}
	if m.cfg.Migrated && ctx.Log != nil {
		ctx.Log.With(log.Engine).Warn("project.aego: input axes were converted to input actions; open Project Settings → Input and save to rewrite the file")
	}
	m.keys = UserKeys{Store: ctx.Settings}
	if ctx.Settings == nil {
		return nil
	}
	if err := m.registerSettings(ctx); err != nil {
		return err
	}
	ctx.Settings.AfterLoad(m.migrateUserFile)
	return nil
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil {
		return nil
	}
	s := NewService(m.cfg, m.keys)
	s.SetRecorder(ctx.World.Recorder)
	ctx.World.SetService(ServiceID, s)
	ctx.World.SetView(inputactions.ReaderID, inputactions.Reader(s))
	return nil
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	if s := Of(ctx.World); s != nil {
		s.Close()
	}
	ctx.World.RemoveService(ServiceID)
	ctx.World.RemoveService(inputactions.ReaderID)
}

func Of(w *engine.World) *Service {
	if w == nil {
		return nil
	}
	s, _ := engine.ServiceOf[*Service](w, ServiceID)
	return s
}

type frameSystem struct{}

func (frameSystem) Run(ctx *engine.Context) {
	s := Of(ctx.World)
	if s == nil {
		return
	}
	ck := Clock{Dt: ctx.Unscaled, Next: ctx.Tick + 1}
	if t := ctx.Time; t != nil {
		r := t.Span(clock.Real)
		ck.Real, ck.Dt = r.Elapsed, r.Delta
		ck.Game = t.Span(clock.Game).Elapsed
		ck.Next = t.Tick() + 1
	}
	s.Frame(ctx.Input(), ck)
}

type tickSystem struct{}

func (tickSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.Tick()
	}
}

type tickEndSystem struct{}

func (tickEndSystem) Run(ctx *engine.Context) {
	if s := Of(ctx.World); s != nil {
		s.EndTick()
	}
}
