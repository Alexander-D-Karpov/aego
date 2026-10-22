package physics

import (
	"aego/contracts/physics2d"
	"aego/engine"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/physics/settings"
	"aego/modules/physics/stdlib"
	"aego/modules/physics/systems"
	"aego/scene"
)

const (
	ID              = "physics"
	SystemSync      = ID + ".sync"
	SystemStep      = ID + ".step"
	SystemWriteback = ID + ".writeback"
	SystemEvents    = ID + ".events"
	SystemPresent   = ID + ".present"
	Menu            = "Physics"
	problemsService = ID + ".problems"
)

type Module struct {
	sections *module.Sections
	problems *problems.Service
	lib      *stdlib.Lib
	events   stdlib.EventIDs
}

func New() *Module {
	return &Module{}
}

func (*Module) Info() module.Info {
	return module.Info{ID: ID, Version: engine.Version, Builtin: true, DisplayName: "Physics 2D", DocsOrder: 30, DocsIcon: "cat.physics",
		Codes: []string{systems.CodeScale, systems.CodeGeometry, systems.CodeBody, systems.CodeJoint, systems.CodeSpace, systems.CodeReparent, systems.CodeUpdate}}
}

func (*Module) IconDir() string {
	return "modules/physics/res/icons/src"
}

func (m *Module) Register(r *module.Registry) error {
	if err := registerTypes(r); err != nil {
		return err
	}
	if b := r.Bindings(); b != nil {
		m.lib = stdlib.Install(b)
	}
	m.events = stdlib.RegisterEvents(r)
	r.ProjectSection(settings.Section())
	registerTemplates(r)
	registerSystems(r, &stdlib.Emitter{IDs: m.events, Lib: m.lib})
	registerDocs(r)
	return nil
}

func (m *Module) Init(ctx *module.InitContext) error {
	m.sections = ctx.Sections
	m.problems = ctx.Problems
	return nil
}

func (m *Module) Project() *settings.Project {
	if p, ok := module.SectionValue[settings.Project](m.sections, settings.ID); ok {
		return p
	}
	d := settings.Default()
	return &d
}

func (m *Module) AttachWorld(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil || ctx.Scene == nil {
		return nil
	}
	p := m.Project()
	sim, err := systems.New(ctx.Scene, p.Config(), p.Matrix(), ctx.Log)
	if err != nil {
		return err
	}
	sim.SetLayers(p.Layers())
	ctx.World.SetService(systems.ServiceID, sim)
	ctx.World.SetView(physics2d.ServiceID, physics2d.Queries(sim))
	if m.problems != nil {
		scn := ctx.Scene
		drop := m.problems.Add(problems.Source{
			Revision: func() uint64 {
				_, r := sim.Diagnostics()
				return r
			},
			Append: func(dst []problems.Diagnostic) []problems.Diagnostic {
				return appendSim(dst, sim, scn)
			},
		})
		ctx.World.SetView(problemsService, drop)
	}
	return nil
}

func (m *Module) Reconfigure(ctx *module.AttachContext) error {
	if !ctx.Play || ctx.World == nil {
		return nil
	}
	sim := SimOf(ctx.World)
	if sim == nil {
		return nil
	}
	p := m.Project()
	if err := sim.World().SetConfig(p.Config()); err != nil {
		return err
	}
	sim.SetLayers(p.Layers())
	return sim.World().SetLayerMatrix(p.Matrix())
}

func (m *Module) DetachWorld(ctx *module.AttachContext) {
	if !ctx.Play || ctx.World == nil {
		return
	}
	if drop, ok := engine.ServiceOf[func()](ctx.World, problemsService); ok {
		drop()
		ctx.World.RemoveService(problemsService)
	}
	sim := SimOf(ctx.World)
	ctx.World.RemoveService(physics2d.ServiceID)
	ctx.World.RemoveService(systems.ServiceID)
	if sim != nil && sim.Scene() == ctx.Scene {
		sim.Close()
	}
}

func appendSim(dst []problems.Diagnostic, sim *systems.Sim, scn *scene.Scene) []problems.Diagnostic {
	ds, _ := sim.Diagnostics()
	for _, d := range ds {
		sev := problems.Warning
		if d.Error {
			sev = problems.Error
		}
		kind := problems.SubjectNode
		if d.Component != "" {
			kind = problems.SubjectField
		}
		dst = append(dst, problems.Diagnostic{
			Owner: ID, Code: d.Code, Severity: sev, Message: d.Message,
			Subject: problems.Subject{Kind: kind, Scene: scn, Node: d.Node, Field: d.Component},
		})
	}
	return dst
}

func SimOf(w *engine.World) *systems.Sim {
	s, _ := engine.ServiceOf[*systems.Sim](w, systems.ServiceID)
	return s
}

type simSystem func(*systems.Sim, *engine.Context)

func (f simSystem) Run(ctx *engine.Context) {
	if s := SimOf(ctx.World); s != nil {
		f(s, ctx)
	}
}

func registerSystems(r *module.Registry, events engine.System) {
	r.System(engine.SystemDesc{
		ID: SystemSync, Phase: engine.PhasePhysics, Scope: engine.ScopePlay,
		System: simSystem(func(s *systems.Sim, _ *engine.Context) { s.Sync() }),
	})
	r.System(engine.SystemDesc{
		ID: SystemStep, Phase: engine.PhasePhysics, Scope: engine.ScopePlay, After: []string{SystemSync},
		System: simSystem(func(s *systems.Sim, ctx *engine.Context) { s.Step(float32(ctx.Dt)) }),
	})
	r.System(engine.SystemDesc{
		ID: SystemWriteback, Phase: engine.PhasePhysics, Scope: engine.ScopePlay, After: []string{SystemStep},
		System: simSystem(func(s *systems.Sim, _ *engine.Context) { s.Writeback() }),
	})
	r.System(engine.SystemDesc{
		ID: SystemEvents, Phase: engine.PhasePhysics, Scope: engine.ScopePlay, After: []string{SystemWriteback},
		System: events,
	})
	r.System(engine.SystemDesc{
		ID: SystemDebug, Phase: engine.PhasePostPhysics, Scope: engine.ScopePlay,
		System: debugSystem{sink: &drawSink{}},
	})
	r.System(engine.SystemDesc{
		ID: SystemPresent, Phase: engine.PhaseAnimation, Scope: engine.ScopePlay, Pause: engine.PauseAlways,
		System: simSystem(func(s *systems.Sim, ctx *engine.Context) { s.Present(float32(ctx.Alpha)) }),
	})
}
