package module

import (
	"errors"
	"strings"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/scene"
	"aego/schema"
)

const EngineModuleID = "engine.modules"

type Manifest struct {
	Modules      []Info
	Systems      []engine.SystemDesc
	Templates    []schema.TemplateDesc
	Icons        []IconSet
	Docs         []DocSource
	ScriptOwners map[string]string
	Dev          []Hook
	Editor       []Hook
	Warnings     []string
	order        []*entry
	services     []serviceDesc
	engines      []engine.Module
}

func (r *Registry) Build() (*Manifest, error) {
	if r.built {
		return nil, errs.New(errs.EngineModule, "module registry is already built")
	}
	r.built = true
	order, err := r.sort()
	if err != nil {
		return nil, err
	}
	for _, e := range order {
		r.cur = e
		if err := e.mod.Register(r); err != nil {
			r.wrap(err, "module "+e.info.ID+" failed to register")
		}
		r.cur = nil
	}
	for _, f := range r.finals {
		if err := f.fn(r); err != nil {
			r.errs = append(r.errs, err)
		}
	}
	icons := r.scanIcons()
	r.checkComponents(icons)
	r.checkSystems()
	r.checkServices()
	r.checkTemplates(icons)
	if len(r.errs) > 0 {
		return nil, errors.Join(r.errs...)
	}
	return r.manifest(order), nil
}

func (r *Registry) sort() ([]*entry, error) {
	byID := make(map[string]*entry, len(r.modules))
	for _, e := range r.modules {
		byID[e.info.ID] = e
	}
	var missing []error
	for _, e := range r.modules {
		for _, dep := range e.info.Requires {
			if _, ok := byID[dep]; !ok {
				missing = append(missing, errs.New(errs.EngineModule,
					"module "+e.info.ID+" requires "+dep+", which is not registered", log.F("module", e.info.ID)))
			}
		}
	}
	if len(missing) > 0 {
		return nil, errors.Join(missing...)
	}
	done := make(map[string]bool, len(r.modules))
	out := make([]*entry, 0, len(r.modules))
	for len(out) < len(r.modules) {
		var next *entry
		for _, e := range r.modules {
			if done[e.info.ID] {
				continue
			}
			ready := true
			for _, dep := range e.info.Requires {
				if !done[dep] {
					ready = false
					break
				}
			}
			if ready {
				next = e
				break
			}
		}
		if next == nil {
			var cyc []string
			for _, e := range r.modules {
				if !done[e.info.ID] {
					cyc = append(cyc, e.info.ID)
				}
			}
			return nil, errs.New(errs.EngineModule, "module requirements form a cycle: "+strings.Join(cyc, ", "))
		}
		done[next.info.ID] = true
		out = append(out, next)
	}
	return out, nil
}

func (r *Registry) checkComponents(icons map[string]bool) {
	sr := r.Schema()
	for _, d := range r.comps {
		for _, name := range d.Requires {
			td, ok := sr.ByName(name)
			if !ok || td.Kind != schema.TypeComponent {
				r.report(d.Owner, "component "+d.FullName+" requires "+name+", which is not a registered component")
			}
		}
		r.useIcon(icons, d.Owner, d.Icon, "component "+d.FullName)
		if t := d.Template; t != nil {
			if t.Category == "" {
				t.Category = d.Category
			}
			if t.Icon == "" {
				t.Icon = d.Icon
			}
		}
	}
}

func reserved(p engine.Phase) bool {
	return p == engine.PhaseFrameBegin || p == engine.PhasePollEvents || p == engine.PhaseFrameEnd
}

func (r *Registry) checkSystems() {
	for _, s := range r.systems {
		id := s.owner.info.ID
		if !strings.HasPrefix(s.desc.ID, id+".") {
			r.report(id, "system "+s.desc.ID+" must be named "+id+".<name>")
		}
		if !s.owner.info.Builtin && reserved(s.desc.Phase) {
			r.report(id, "system "+s.desc.ID+" uses the reserved phase "+s.desc.Phase.String())
		}
	}
}

func (r *Registry) checkServices() {
	seen := map[string]string{}
	for _, s := range r.services {
		if other, dup := seen[s.id]; dup {
			r.report(s.owner, "service "+s.id+" is already registered by "+other)
			continue
		}
		seen[s.id] = s.owner
	}
}

func (r *Registry) checkTemplates(icons map[string]bool) {
	seen := map[string]string{}
	for _, t := range r.templates {
		if t.Name == "" {
			r.report(t.Owner, "template needs a name")
			continue
		}
		if t.ID == "" {
			t.ID = t.Owner + "." + strings.ToLower(t.Name)
		}
		if other, dup := seen[t.ID]; dup {
			r.report(t.Owner, "template "+t.ID+" is already registered by "+other)
			continue
		}
		seen[t.ID] = t.Owner
		if _, ok := t.Setup.(func(*scene.Scene, scene.NodeHandle)); !ok {
			r.report(t.Owner, "template "+t.ID+" needs Setup func(*scene.Scene, scene.NodeHandle)")
		}
		r.useIcon(icons, t.Owner, t.Icon, "template "+t.ID)
	}
}

func (r *Registry) manifest(order []*entry) *Manifest {
	m := &Manifest{
		order: order, services: r.services, engines: r.engines,
		Icons: r.icons, Docs: r.docs, ScriptOwners: r.owners,
		Dev: r.dev, Editor: r.editor, Warnings: r.warns,
	}
	for _, e := range order {
		m.Modules = append(m.Modules, e.info)
	}
	for _, s := range r.systems {
		m.Systems = append(m.Systems, s.desc)
	}
	for _, t := range r.templates {
		m.Templates = append(m.Templates, *t)
	}
	return m
}

type engineModule struct {
	m *Manifest
}

func (e engineModule) Info() engine.ModuleInfo {
	return engine.ModuleInfo{ID: EngineModuleID, Version: engine.Version}
}

func (e engineModule) Register(reg *engine.Registry) error {
	for _, d := range e.m.Systems {
		if err := reg.Systems().Register(d); err != nil {
			return err
		}
	}
	for _, m := range e.m.engines {
		if err := m.Register(reg); err != nil {
			id := m.Info().ID
			return errs.Wrap(err, errs.EngineModule, "engine module "+id+" failed to register").With(log.F("module", id))
		}
	}
	return nil
}

func (e engineModule) Init(eng *engine.Engine) error {
	for _, m := range e.m.engines {
		if in, ok := m.(engine.ModuleIniter); ok {
			if err := in.Init(eng); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manifest) Engine() engine.Module {
	return engineModule{m: m}
}

func (m *Manifest) Init(ctx *InitContext) error {
	for _, s := range m.services {
		ctx.Engine.SetService(s.id, s.value)
	}
	for _, e := range m.order {
		in, ok := e.mod.(Initer)
		if !ok {
			continue
		}
		if err := in.Init(ctx); err != nil {
			return errs.Wrap(err, errs.EngineModule, "module "+e.info.ID+" failed to initialize").With(log.F("module", e.info.ID))
		}
	}
	return nil
}

func (m *Manifest) Attach(ctx *AttachContext) (func(), error) {
	var undo []func()
	detach := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}
	for _, e := range m.order {
		at, ok := e.mod.(Attacher)
		if !ok {
			continue
		}
		d, err := at.Attach(ctx)
		if err != nil {
			detach()
			return nil, errs.Wrap(err, errs.EngineModule, "module "+e.info.ID+" failed to attach").With(log.F("module", e.info.ID))
		}
		if d != nil {
			undo = append(undo, d)
		}
	}
	return detach, nil
}
