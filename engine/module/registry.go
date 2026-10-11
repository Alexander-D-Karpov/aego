package module

import (
	"io/fs"

	"aego/core/errs"
	"aego/core/log"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

type entry struct {
	mod  Module
	info Info
}

type ownedSystem struct {
	desc  engine.SystemDesc
	owner *entry
}

type serviceDesc struct {
	id    string
	value any
	owner string
}

type finalizer struct {
	owner string
	fn    func(*Registry) error
}

type Registry struct {
	types     *scene.Types
	script    *bind.Registry
	modules   []*entry
	cur       *entry
	comps     []*schema.TypeDescriptor
	systems   []ownedSystem
	engines   []engine.Module
	services  []serviceDesc
	templates []*schema.TemplateDesc
	icons     []IconSet
	docs      []fs.FS
	dev       []Hook
	editor    []Hook
	finals    []finalizer
	warns     []string
	errs      []error
	built     bool
}

func New(types *scene.Types, script *bind.Registry) *Registry {
	return &Registry{types: types, script: script}
}

func (r *Registry) Types() *scene.Types {
	return r.types
}

func (r *Registry) Schema() *schema.Registry {
	return r.types.Schema()
}

func (r *Registry) Bindings() *bind.Registry {
	return r.script
}

func (r *Registry) Components() []*schema.TypeDescriptor {
	return r.comps
}

func (r *Registry) Owner() string {
	if r.cur == nil {
		return ""
	}
	return r.cur.info.ID
}

func (r *Registry) builtin() bool {
	return r.cur != nil && r.cur.info.Builtin
}

func (r *Registry) fail(msg string) error {
	err := errs.New(errs.EngineModule, msg, log.F("module", r.Owner()))
	r.errs = append(r.errs, err)
	return err
}

func (r *Registry) wrap(err error, msg string) error {
	var w error = errs.Wrap(err, errs.EngineModule, msg).With(log.F("module", r.Owner()))
	r.errs = append(r.errs, w)
	return w
}

func (r *Registry) Warn(owner, msg string) {
	r.warns = append(r.warns, "module "+owner+": "+msg)
}

func (r *Registry) report(owner, msg string) {
	r.errs = append(r.errs, errs.New(errs.EngineModule, msg, log.F("module", owner)))
}

func (r *Registry) Add(m Module) error {
	if r.built {
		return errs.New(errs.EngineModule, "module registry is already built")
	}
	info := m.Info()
	if info.ID == "" {
		return errs.New(errs.EngineModule, "module needs an id")
	}
	for _, e := range r.modules {
		if e.info.ID == info.ID {
			return errs.New(errs.EngineModule, "module "+info.ID+" is registered twice", log.F("module", info.ID))
		}
	}
	r.modules = append(r.modules, &entry{mod: m, info: info})
	return nil
}

func (r *Registry) System(d engine.SystemDesc) {
	if r.cur == nil {
		r.fail("system " + d.ID + " is registered outside a module")
		return
	}
	r.systems = append(r.systems, ownedSystem{desc: d, owner: r.cur})
}

func (r *Registry) Use(m engine.Module) {
	if m == nil {
		r.fail("engine module is nil")
		return
	}
	r.engines = append(r.engines, m)
}

func (r *Registry) Service(id string, v any) {
	if id == "" || v == nil {
		r.fail("service needs an id and a value")
		return
	}
	r.services = append(r.services, serviceDesc{id: id, value: v, owner: r.Owner()})
}

func (r *Registry) ScriptModule(path string) *bind.Module {
	if r.script == nil {
		r.fail("script module " + path + " needs script bindings")
		return bind.New().Module(path)
	}
	return r.script.Module(path)
}

func (r *Registry) Template(t schema.TemplateDesc) {
	r.addTemplate(&t)
}

func (r *Registry) addTemplate(t *schema.TemplateDesc) {
	t.Owner = r.Owner()
	t.Builtin = r.builtin()
	r.templates = append(r.templates, t)
}

func (r *Registry) Icons(fsys fs.FS, prefix string) {
	if fsys == nil {
		r.fail("icon source is nil")
		return
	}
	r.icons = append(r.icons, IconSet{Owner: r.Owner(), FS: fsys, Prefix: prefix})
}

func (r *Registry) Docs(fsys fs.FS) {
	if fsys == nil {
		r.fail("docs source is nil")
		return
	}
	r.docs = append(r.docs, fsys)
}

func (r *Registry) Dev(fn any) {
	if fn == nil {
		r.fail("dev hook is nil")
		return
	}
	r.dev = append(r.dev, Hook{Owner: r.Owner(), Fn: fn})
}

func (r *Registry) Editor(fn any) {
	if fn == nil {
		r.fail("editor hook is nil")
		return
	}
	r.editor = append(r.editor, Hook{Owner: r.Owner(), Fn: fn})
}

func (r *Registry) Finalize(fn func(*Registry) error) {
	if fn == nil {
		r.fail("finalizer is nil")
		return
	}
	r.finals = append(r.finals, finalizer{owner: r.Owner(), fn: fn})
}
