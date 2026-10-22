package ext

import (
	"aego/editor/ext/inspect"
	"fmt"
	"runtime/debug"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/gizmo"
	"aego/editor/core/shape"
	"aego/editor/core/templates"
	"aego/editor/core/workspace"
	"aego/scene"
	"aego/schema"
)

type Deps struct {
	Editor     *context.Editor
	Workspace  *workspace.Workspace
	Inspectors *inspect.Registry
	Gizmos     *gizmo.Registry
	Templates  *templates.Registry
	Shapes     *shape.Registry
	Commands   *command.Registry
	Schema     *schema.Registry
}

type owned struct {
	panels     []string
	commands   []string
	gizmos     []schema.TypeID
	inspectors []schema.TypeID
	assets     []string
	players    []string
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
	ticks    []tick
}

type tick struct {
	ext string
	fn  func()
}

// Tick runs fn once per editor frame, before the panels draw, whether or not any view is visible.
func (r *Registry) Tick(fn func()) {
	r.ticks = append(r.ticks, tick{ext: r.ext(), fn: fn})
}

// RunTicks calls the Tick hooks of the extensions that are not disabled.
func (r *Registry) RunTicks() {
	for _, t := range r.ticks {
		if _, off := r.disabled[t.ext]; off {
			continue
		}
		Safe(t.ext, r, t.fn)
	}
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

func (r *Registry) Inspectors() *inspect.Registry {
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

func (r *Registry) Run(owner string, fn any) error {
	h, ok := fn.(func(*Registry) error)
	if !ok {
		err := errs.New(errs.EditorPanel, "editor hook of "+owner+" has the unsupported type "+fmt.Sprintf("%T", fn), log.F("module", owner))
		r.disableNow(owner, err.Error())
		return err
	}
	r.current = owner
	defer func() { r.current = "" }()
	if err := safeCall(func() error { return h(r) }); err != nil {
		r.disableNow(owner, err.Error())
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
	if ed := r.deps.Editor; ed != nil && ed.Crash != nil {
		ed.Crash("extension "+id, reason, debug.Stack())
	}
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
		for _, c := range o.commands {
			r.deps.Commands.Unregister(c)
		}
		for _, g := range o.gizmos {
			r.deps.Gizmos.Unregister(g)
		}
		for _, t := range o.inspectors {
			r.deps.Inspectors.Disable(t, reason)
		}
		if ed := r.deps.Editor; ed != nil {
			for _, k := range o.assets {
				delete(ed.AssetViews, k)
			}
			for _, k := range o.players {
				delete(ed.AssetPlayers, k)
			}
		}
	}
	if r.deps.Workspace != nil {
		r.deps.Workspace.RemovePresets(id)
	}
	if r.deps.Shapes != nil {
		r.deps.Shapes.Remove(id)
	}
	if r.deps.Templates != nil {
		r.deps.Templates.RemoveDrops(id)
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

func (r *Registry) typeOf(fullName string) (*schema.TypeDescriptor, error) {
	td, ok := r.deps.Schema.ByName(fullName)
	if !ok || td.Kind != schema.TypeComponent {
		return nil, errs.New(errs.SchemaUnknownType, "component is not registered", log.F("type", fullName), log.F("module", r.ext()))
	}
	return td, nil
}

const extOrder = 500

func (r *Registry) Panel(d workspace.Desc) error {
	if d.Panel == nil {
		return errs.New(errs.EditorPanel, "panel descriptor has no panel", log.F("module", r.ext()))
	}
	ext := r.ext()
	id := d.Panel.ID()
	d.Panel = &safePanel{p: d.Panel, ext: ext, reg: r}
	if d.Order == 0 {
		d.Order = extOrder
	}
	if err := r.deps.Workspace.Register(d); err != nil {
		return err
	}
	o := r.own(ext)
	o.panels = append(o.panels, id)
	return nil
}

func (r *Registry) Inspector(fullName string, c inspect.Custom) error {
	td, err := r.typeOf(fullName)
	if err != nil {
		return err
	}
	ext := r.ext()
	if err := r.deps.Inspectors.RegisterCustomID(td.ID, &safeCustom{inner: c, ext: ext, reg: r}); err != nil {
		return err
	}
	o := r.own(ext)
	o.inspectors = append(o.inspectors, td.ID)
	return nil
}

func (r *Registry) Gizmo(fullName string, g gizmo.Gizmo) error {
	td, err := r.typeOf(fullName)
	if err != nil {
		return err
	}
	ext := r.ext()
	if err := r.deps.Gizmos.Register(td.ID, &safeGizmo{inner: g, ext: ext, reg: r}); err != nil {
		return err
	}
	o := r.own(ext)
	o.gizmos = append(o.gizmos, td.ID)
	return nil
}

func (r *Registry) Bounds(fullName string, fn shape.Func) error {
	td, err := r.typeOf(fullName)
	if err != nil {
		return err
	}
	ext := r.ext()
	return r.deps.Shapes.Register(ext, td.ID, func(s *scene.Scene, res asset.Resolver, h scene.NodeHandle) (sh shape.Shape, ok bool) {
		defer func() {
			if rec := recover(); rec != nil {
				r.Disable(ext, fmt.Sprint(rec))
				sh, ok = shape.Shape{}, false
			}
		}()
		return fn(s, res, h)
	})
}

func (r *Registry) Overlay(fn shape.Overlay) {
	r.overlay(fn, false)
}

func (r *Registry) GameOverlay(fn shape.Overlay) {
	r.overlay(fn, true)
}

func (r *Registry) overlay(fn shape.Overlay, game bool) {
	ext := r.ext()
	r.deps.Shapes.AddOverlayIn(ext, func(c *shape.OverlayCtx) {
		Safe(ext, r, func() { fn(c) })
	}, game)
}

func (r *Registry) AssetDrop(kind string, space scene.Space, setup func(*scene.Scene, scene.NodeHandle, ids.AssetID)) error {
	ext := r.ext()
	return r.deps.Templates.AddDrop(templates.Drop{
		Owner: ext,
		Kind:  kind,
		Space: space,
		Setup: func(s *scene.Scene, h scene.NodeHandle, id ids.AssetID) {
			defer func() {
				if rec := recover(); rec != nil {
					r.Disable(ext, fmt.Sprint(rec))
					panic(rec)
				}
			}()
			setup(s, h, id)
		},
	})
}

func (r *Registry) Command(d command.Desc) error {
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
