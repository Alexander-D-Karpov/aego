package extension

import (
	"fmt"
	"reflect"

	"aego/core/errs"
	"aego/core/log"
	"aego/editor/command"
	"aego/editor/context"
	"aego/editor/gizmo"
	"aego/editor/inspector"
	"aego/editor/templates"
	"aego/editor/workspace"
	"aego/engine"
	"aego/scene"
	"aego/schema"
)

type Module interface {
	RegisterEditor(*Registry) error
}

type named interface {
	Info() engine.ModuleInfo
}

func ModuleID(m any) string {
	if n, ok := m.(named); ok && n.Info().ID != "" {
		return n.Info().ID
	}
	return reflect.TypeOf(m).String()
}

type Deps struct {
	Editor     *context.Editor
	Workspace  *workspace.Workspace
	Inspectors *inspector.Registry
	Gizmos     *gizmo.Registry
	Templates  *templates.Registry
	Commands   *command.Registry
	Schema     *schema.Registry
}

type owned struct {
	panels     []string
	templates  []string
	commands   []string
	gizmos     []schema.TypeID
	inspectors []schema.TypeID
}

type pending struct {
	id     string
	reason string
}

type Registry struct {
	deps     Deps
	current  string
	owned    map[string]*owned
	disabled map[string]string
	pending  []pending
}

func New(d Deps) *Registry {
	return &Registry{deps: d, owned: map[string]*owned{}, disabled: map[string]string{}}
}

func (r *Registry) Editor() *context.Editor {
	return r.deps.Editor
}

func (r *Registry) Schema() *schema.Registry {
	return r.deps.Schema
}

func (r *Registry) Inspectors() *inspector.Registry {
	return r.deps.Inspectors
}

func safeCall(fn func() error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = errs.New(errs.EditorPanel, "extension panicked", log.F("panic", fmt.Sprint(rec)))
		}
	}()
	return fn()
}

func (r *Registry) Load(m Module) error {
	id := ModuleID(m)
	r.current = id
	defer func() { r.current = "" }()
	if err := safeCall(func() error { return m.RegisterEditor(r) }); err != nil {
		r.disableNow(id, err.Error())
		return err
	}
	return nil
}

func (r *Registry) ext() string {
	if r.current == "" {
		return "editor"
	}
	return r.current
}

func (r *Registry) own(id string) *owned {
	o := r.owned[id]
	if o == nil {
		o = &owned{}
		r.owned[id] = o
	}
	return o
}

func (r *Registry) Disable(id, reason string) {
	r.pending = append(r.pending, pending{id: id, reason: reason})
}

func (r *Registry) Flush() {
	list := r.pending
	r.pending = nil
	for _, p := range list {
		r.disableNow(p.id, p.reason)
	}
}

func (r *Registry) disableNow(id, reason string) {
	if _, done := r.disabled[id]; done {
		return
	}
	r.disabled[id] = reason
	if o := r.owned[id]; o != nil {
		for _, p := range o.panels {
			r.deps.Workspace.Unregister(p)
		}
		for _, t := range o.templates {
			r.deps.Templates.Unregister(t)
		}
		for _, c := range o.commands {
			r.deps.Commands.Unregister(c)
		}
		for _, g := range o.gizmos {
			r.deps.Gizmos.Unregister(g)
		}
		for _, t := range o.inspectors {
			r.deps.Inspectors.Disable(t, reason)
		}
	}
	if r.deps.Editor != nil {
		r.deps.Editor.Notify(context.LevelError, "extension "+id+" was disabled: "+reason)
	}
}

func (r *Registry) Disabled() map[string]string {
	return r.disabled
}

func Safe(id string, r *Registry, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			r.Disable(id, fmt.Sprint(rec))
		}
	}()
	fn()
}

func (r *Registry) RegisterPanel(p workspace.Panel) error {
	ext := r.ext()
	if err := r.deps.Workspace.Register(&safePanel{inner: p, ext: ext, reg: r}); err != nil {
		return err
	}
	o := r.own(ext)
	o.panels = append(o.panels, p.ID())
	return nil
}

func (r *Registry) RegisterInspector(t schema.TypeID, c inspector.Custom) error {
	ext := r.ext()
	if err := r.deps.Inspectors.RegisterCustomID(t, &safeCustom{inner: c, ext: ext, reg: r}); err != nil {
		return err
	}
	o := r.own(ext)
	o.inspectors = append(o.inspectors, t)
	return nil
}

func (r *Registry) RegisterGizmo(t schema.TypeID, g gizmo.Gizmo) error {
	ext := r.ext()
	if err := r.deps.Gizmos.Register(t, &safeGizmo{inner: g, ext: ext, reg: r}); err != nil {
		return err
	}
	o := r.own(ext)
	o.gizmos = append(o.gizmos, t)
	return nil
}

func (r *Registry) RegisterTemplate(t templates.Template) error {
	ext := r.ext()
	if t.Module == "" {
		t.Module = ext
	}
	build := t.Build
	if build == nil {
		return errs.New(errs.EditorPanel, "template needs a build function", log.F("template", t.ID))
	}
	t.Build = func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
		defer func() {
			if rec := recover(); rec != nil {
				r.Disable(ext, fmt.Sprint(rec))
				panic(rec)
			}
		}()
		return build(s, parent)
	}
	if err := r.deps.Templates.Register(t); err != nil {
		return err
	}
	o := r.own(ext)
	o.templates = append(o.templates, t.ID)
	return nil
}

func (r *Registry) RegisterCommand(d command.Desc) error {
	ext := r.ext()
	run, enabled := d.Run, d.Enabled
	if run == nil {
		return errs.New(errs.EditorCommand, "command needs a run function", log.F("command", d.ID))
	}
	d.Run = func() (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				r.Disable(ext, fmt.Sprint(rec))
				err = errs.New(errs.EditorCommand, "command panicked", log.F("command", d.ID), log.F("panic", fmt.Sprint(rec)))
			}
		}()
		return run()
	}
	if enabled != nil {
		d.Enabled = func() (ok bool) {
			defer func() {
				if rec := recover(); rec != nil {
					r.Disable(ext, fmt.Sprint(rec))
					ok = false
				}
			}()
			return enabled()
		}
	}
	if d.Category == "" {
		d.Category = ext
	}
	if err := r.deps.Commands.Register(d); err != nil {
		return err
	}
	o := r.own(ext)
	o.commands = append(o.commands, d.ID)
	return nil
}

func typeID[T any](r *Registry) (schema.TypeID, error) {
	rt := reflect.TypeFor[T]()
	td, ok := r.deps.Schema.ByGoType(rt)
	if !ok {
		return 0, errs.New(errs.SchemaUnknownType, "type is not registered as a component", log.F("type", rt.String()))
	}
	return td.ID, nil
}

func RegisterInspector[T any](r *Registry, c inspector.Custom) error {
	t, err := typeID[T](r)
	if err != nil {
		return err
	}
	return r.RegisterInspector(t, c)
}

func RegisterGizmo[T any](r *Registry, g gizmo.Gizmo) error {
	t, err := typeID[T](r)
	if err != nil {
		return err
	}
	return r.RegisterGizmo(t, g)
}
