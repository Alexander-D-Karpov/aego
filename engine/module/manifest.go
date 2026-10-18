package module

import (
	"errors"
	"slices"
	"strings"

	"aego/contracts/scriptevents"
	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/engine/settings"
	"aego/scene"
	"aego/schema"
)

const EngineModuleID = "engine.modules"

type Manifest struct {
	Modules         []Info
	Systems         []engine.SystemDesc
	Templates       []schema.TemplateDesc
	Icons           []IconSet
	Docs            []DocSource
	Events          []scriptevents.Event
	Sections        []*Section
	ScriptOwners    map[string]string
	Dev             []Hook
	Editor          []Hook
	Warnings        []string
	Settings        []settings.Desc
	SettingVersions []settings.OwnerVersion
	order           []*entry
	services        []serviceDesc
	engines         []ownedEngine
	inited          []*entry
	provided        []provision
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
	r.checkSections()
	r.checkSettings()
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
			return nil, errs.New(errs.EngineModule, "module requirements form a cycle: "+strings.Join(r.cycle(byID, done), " -> "))
		}
		done[next.info.ID] = true
		out = append(out, next)
	}
	return out, nil
}

func (r *Registry) cycle(byID map[string]*entry, done map[string]bool) []string {
	state := map[string]uint8{}
	var stack, found []string
	var visit func(id string) bool
	visit = func(id string) bool {
		state[id] = 1
		stack = append(stack, id)
		for _, dep := range byID[id].info.Requires {
			if done[dep] {
				continue
			}
			switch state[dep] {
			case 1:
				found = append(slices.Clone(stack[slices.Index(stack, dep):]), dep)
				return true
			case 0:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return false
	}
	for _, e := range r.modules {
		if !done[e.info.ID] && state[e.info.ID] == 0 && visit(e.info.ID) {
			return found
		}
	}
	return nil
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

func (r *Registry) checkSections() {
	seen := map[string]string{}
	for _, s := range r.sections {
		if s.ID != s.Owner && !strings.HasPrefix(s.ID, s.Owner+".") {
			r.report(s.Owner, "project section "+s.ID+" must be named "+s.Owner+" or "+s.Owner+".<name>")
		}
		for _, h := range s.Heads() {
			if other, dup := seen[h]; dup {
				r.report(s.Owner, "project section "+h+" is already registered by "+other)
				continue
			}
			seen[h] = s.Owner
		}
	}
}

func (r *Registry) manifest(order []*entry) *Manifest {
	m := &Manifest{
		order: order, services: r.services, engines: r.engines, provided: r.provided,
		Icons: r.icons, Docs: r.docs, ScriptOwners: r.owners,
		Dev: r.dev, Editor: r.editor, Warnings: r.warns,
		Events: r.Events(), Sections: r.sections,
		Settings: r.settings, SettingVersions: r.versions,
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
	for _, oe := range e.m.engines {
		id := oe.m.Info().ID
		before := len(reg.Systems().IDs())
		if err := oe.m.Register(reg); err != nil {
			return errs.Wrap(err, errs.EngineModule, "engine module "+id+" failed to register").With(log.F("module", oe.owner))
		}
		for _, sid := range reg.Systems().IDs()[before:] {
			if !strings.HasPrefix(sid, oe.owner+".") {
				return errs.New(errs.EngineModule, "system "+sid+" of engine module "+id+" must be named "+oe.owner+".<name>", log.F("module", oe.owner))
			}
		}
	}
	return nil
}

func (e engineModule) Init(eng *engine.Engine) error {
	for _, oe := range e.m.engines {
		if in, ok := oe.m.(engine.ModuleIniter); ok {
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

func (r *Registry) checkSettings() {
	seen := map[string]string{}
	for _, d := range r.settings {
		if !strings.HasPrefix(d.ID, d.Owner+".") || !settings.ValidPath(d.ID) {
			r.report(d.Owner, "setting "+d.ID+" must be named "+d.Owner+".<name>")
		}
		if other, dup := seen[d.ID]; dup {
			r.report(d.Owner, "setting "+d.ID+" is already registered by "+other)
			continue
		}
		seen[d.ID] = d.Owner
	}
}

func (m *Manifest) NewSettings() (*settings.Store, error) {
	st := settings.New()
	var all []error
	for _, v := range m.SettingVersions {
		if err := st.OwnerVersion(v); err != nil {
			all = append(all, err)
		}
	}
	for _, d := range m.Settings {
		if _, err := st.Register(d); err != nil {
			all = append(all, err)
		}
	}
	return st, errors.Join(all...)
}

func (m *Manifest) Init(ctx *InitContext) error {
	ctx.provided = m.provided
	if ctx.Engine != nil {
		for _, s := range m.services {
			ctx.Engine.SetService(s.id, s.value)
		}
	}
	for _, e := range m.order {
		if in, ok := e.mod.(Initer); ok {
			ctx.owner = e.info.ID
			err := in.Init(ctx)
			ctx.owner = ""
			if err != nil {
				m.Shutdown()
				if ctx.Engine != nil {
					for _, s := range m.services {
						ctx.Engine.MainWorld().RemoveService(s.id)
					}
				}
				return errs.Wrap(err, errs.EngineModule, "module "+e.info.ID+" failed to initialize").With(log.F("module", e.info.ID))
			}
		}
		m.inited = append(m.inited, e)
	}
	return nil
}

func (m *Manifest) Shutdown() {
	for i := len(m.inited) - 1; i >= 0; i-- {
		if sd, ok := m.inited[i].mod.(Shutdowner); ok {
			sd.Shutdown()
		}
	}
	m.inited = nil
}

func (m *Manifest) Attach(ctx *AttachContext) (func(), error) {
	var tried []WorldAttacher
	done := false
	detach := func() {
		if done {
			return
		}
		done = true
		for i := len(tried) - 1; i >= 0; i-- {
			tried[i].DetachWorld(ctx)
		}
	}
	for _, e := range m.order {
		at, ok := e.mod.(WorldAttacher)
		if !ok {
			continue
		}
		tried = append(tried, at)
		if err := at.AttachWorld(ctx); err != nil {
			detach()
			return nil, errs.Wrap(err, errs.EngineModule, "module "+e.info.ID+" failed to attach").With(log.F("module", e.info.ID))
		}
	}
	return detach, nil
}

func (m *Manifest) Reconfigure(ctx *AttachContext) error {
	var all []error
	for _, e := range m.order {
		if rc, ok := e.mod.(Reconfigurer); ok {
			if err := rc.Reconfigure(ctx); err != nil {
				all = append(all, errs.Wrap(err, errs.EngineModule, "module "+e.info.ID+" cannot apply the new settings").With(log.F("module", e.info.ID)))
			}
		}
	}
	return errors.Join(all...)
}
