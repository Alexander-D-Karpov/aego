package templates

import (
	"errors"
	"fmt"
	"sort"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/context"
	"aego/editor/core/history/commands"
	"aego/engine/i18n"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

const (
	Category3D  = "3D"
	Moved3DNote = "a 3D node needs a 3D parent; created at the scene root"
)

type Template struct {
	ID       string
	Name     string
	Module   string
	Category string
	Menu     string
	Icon     string
	Builtin  bool
	NoOnAdd  bool
	Space    scene.Space
	Build    func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle
}

type Drop struct {
	Owner string
	Kind  string
	Space scene.Space
	Setup func(s *scene.Scene, h scene.NodeHandle, id ids.AssetID)
}

type Place struct {
	At2 *mathx.Vec2
	At3 *mathx.Vec3
}

func (p Place) Fits(t Template) bool {
	switch {
	case p.At3 != nil:
		return t.Space == scene.Space3D
	case p.At2 != nil:
		return t.Space == scene.Space2D
	}
	return true
}

type PrefabSource interface {
	Space(id ids.AssetID) (scene.Space, bool)
	Build(s *scene.Scene, parent scene.NodeHandle, id ids.AssetID) (scene.NodeHandle, error)
}

type Registry struct {
	list    []Template
	drops   []Drop
	prefabs PrefabSource
}

func (r *Registry) SetPrefabs(p PrefabSource) {
	r.prefabs = p
}

func New() *Registry {
	return &Registry{}
}

func SpaceOf(k schema.SpaceKind) scene.Space {
	if k == schema.Space3D {
		return scene.Space3D
	}
	return scene.Space2D
}

func create(s *scene.Scene, parent scene.NodeHandle, sp scene.Space, name string) scene.NodeHandle {
	if sp == scene.Space3D {
		return s.Create3D(parent, name)
	}
	return s.Create(parent, name)
}

func FromDesc(d schema.TemplateDesc) (Template, error) {
	setup, ok := d.Setup.(func(*scene.Scene, scene.NodeHandle))
	if !ok {
		return Template{}, errs.New(errs.EditorPanel, "template setup has an unsupported type",
			log.F("template", d.ID), log.F("module", d.Owner))
	}
	node := d.Node
	if node == "" {
		node = d.Name
	}
	sp := SpaceOf(d.Space)
	return Template{
		ID: d.ID, Name: d.Name, Module: d.Owner, Category: d.Category, Menu: d.Menu, Icon: d.Icon, Builtin: d.Builtin, Space: sp,
		Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
			h := create(s, parent, sp, node)
			if !h.IsZero() {
				setup(s, h)
			}
			return h
		},
	}, nil
}

func (r *Registry) Load(descs []schema.TemplateDesc) error {
	var all []error
	for _, d := range descs {
		t, err := FromDesc(d)
		if err == nil {
			err = r.Register(t)
		}
		if err != nil {
			all = append(all, err)
		}
	}
	return errors.Join(all...)
}

func (r *Registry) Register(t Template) error {
	if t.ID == "" || t.Build == nil {
		return errs.New(errs.EditorPanel, "template needs an id and a build function", log.F("template", t.ID))
	}
	for _, o := range r.list {
		if o.ID == t.ID {
			return errs.New(errs.EditorPanel, "duplicate template id", log.F("template", t.ID))
		}
	}
	if t.Name == "" {
		t.Name = t.ID
	}
	r.list = append(r.list, t)
	return nil
}

func (r *Registry) Unregister(id string) {
	for i, t := range r.list {
		if t.ID == id {
			r.list = append(r.list[:i], r.list[i+1:]...)
			return
		}
	}
}

func (r *Registry) All() []Template {
	return r.list
}

func (r *Registry) Get(id string) (Template, bool) {
	for _, t := range r.list {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

func (r *Registry) Spawn(s *scene.Scene, parent scene.NodeHandle, id string) scene.NodeHandle {
	t, ok := r.Get(id)
	if !ok {
		return scene.NodeHandle{}
	}
	return t.Build(s, parent)
}

func (r *Registry) Root() []Template {
	var out []Template
	for _, t := range r.list {
		if t.Builtin {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) Modules() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range r.list {
		if !t.Builtin && !seen[t.Module] {
			seen[t.Module] = true
			out = append(out, t.Module)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Registry) InModule(m string) []Template {
	var out []Template
	for _, t := range r.list {
		if !t.Builtin && t.Module == m {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

func (r *Registry) AddDrop(d Drop) error {
	if d.Kind == "" || d.Setup == nil {
		return errs.New(errs.EditorPanel, "asset drop needs a kind and a setup", log.F("module", d.Owner))
	}
	r.drops = append(r.drops, d)
	return nil
}

func (r *Registry) RemoveDrops(owner string) {
	out := r.drops[:0]
	for _, d := range r.drops {
		if d.Owner != owner {
			out = append(out, d)
		}
	}
	r.drops = out
}

func (r *Registry) drop(kind asset.ArtifactKind, space *scene.Space) (Drop, bool) {
	for _, d := range r.drops {
		if context.AssetMatches(d.Kind, kind) && (space == nil || d.Space == *space) {
			return d, true
		}
	}
	return Drop{}, false
}

func (r *Registry) Accepts(kind asset.ArtifactKind, space *scene.Space) bool {
	if kind == asset.ArtPrefab {
		return r.prefabs != nil
	}
	_, ok := r.drop(kind, space)
	return ok
}

func (r *Registry) ForAsset(kind asset.ArtifactKind, space *scene.Space, id ids.AssetID, name string) (Template, bool) {
	if name == "" {
		name = string(kind)
	}
	if kind == asset.ArtPrefab {
		if r.prefabs == nil {
			return Template{}, false
		}
		sp, _ := r.prefabs.Space(id)
		if space != nil && *space != sp {
			return Template{}, false
		}
		src := r.prefabs
		return Template{
			ID: "prefab." + id.String(), Name: name, Module: "editor", Builtin: true, NoOnAdd: true, Space: sp,
			Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
				h, err := src.Build(s, parent, id)
				if err != nil {
					return scene.NodeHandle{}
				}
				return h
			},
		}, true
	}
	d, ok := r.drop(kind, space)
	if !ok {
		return Template{}, false
	}
	return Template{
		ID: d.Owner + ".drop." + string(kind), Name: name, Module: d.Owner, Builtin: true, Space: d.Space,
		Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
			h := create(s, parent, d.Space, name)
			if !h.IsZero() {
				d.Setup(s, h, id)
			}
			return h
		},
	}, true
}

func Parent3D(s *scene.Scene, parent scene.NodeHandle) (scene.NodeHandle, string) {
	if parent.IsZero() || !s.IsAlive(parent) {
		return s.Root(), ""
	}
	if s.AllowsSpace(parent, scene.Space3D) {
		return parent, ""
	}
	return s.Root(), Moved3DNote
}

func resolver(ctx *context.Editor) asset.Resolver {
	if ctx.Loader != nil {
		return ctx.Loader
	}
	return nil
}
func build(t Template, s *scene.Scene) (h scene.NodeHandle, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = errs.New(errs.EditorPanel, "template panicked",
				log.F("template", t.ID), log.F("panic", fmt.Sprint(rec)))
		}
	}()
	h = t.Build(s, s.Root())
	if h.IsZero() || !s.IsAlive(h) {
		err = errs.New(errs.EditorPanel, "template returned no node", log.F("template", t.ID))
	}
	return h, err
}

func place(s, scratch *scene.Scene, h scene.NodeHandle, parent ids.NodeID, pl Place) error {
	var ph scene.NodeHandle
	if !parent.IsZero() {
		ph = s.Handle(parent)
	}
	switch {
	case pl.At3 != nil && scratch.Space(h) == scene.Space3D:
		at := *pl.At3
		if !ph.IsZero() {
			inv, ok := s.World4(ph).InverseAffine()
			if !ok {
				return errs.New(errs.SceneBadReparent, "parent transform cannot be inverted", log.F("parent", s.Name(ph)))
			}
			at = inv.TransformPoint(at)
		}
		t := scratch.Local3D(h)
		t.Position = t.Position.Add(at)
		scratch.SetLocal3D(h, t)
	case pl.At2 != nil && scratch.Space(h) == scene.Space2D:
		local := *pl.At2
		if !ph.IsZero() {
			inv, ok := s.World(ph).Inverse()
			if !ok {
				return errs.New(errs.SceneBadReparent, "parent transform cannot be inverted", log.F("parent", s.Name(ph)))
			}
			local = inv.Apply(local)
		}
		scratch.SetPosition(h, local)
	}
	return nil
}

func InstantiatePlace(ctx *context.Editor, t Template, parent ids.NodeID, index int, pl Place) ([]ids.NodeID, error) {
	d := ctx.Doc()
	if d == nil {
		return nil, errs.New(errs.EditorDocument, "no scene is open")
	}
	s := d.Scene()
	if t.Space == scene.Space3D && !parent.IsZero() {
		if ph := s.Handle(parent); !ph.IsZero() && !s.AllowsSpace(ph, scene.Space3D) {
			parent, index = ids.NodeID{}, -1
			ctx.Notify(context.LevelWarn, Moved3DNote)
		}
	}
	scratch := scene.New(ctx.Types, resolver(ctx), scene.Options{InitialNodes: 16})
	h, err := build(t, scratch)
	if err != nil {
		return nil, err
	}
	scratch.Flush()
	if !t.NoOnAdd {
		for _, n := range scratch.PreOrder() {
			if n != scratch.Root() && scratch.IsAlive(n) {
				ctx.RunOnAdd(scratch, n)
			}
		}
	}
	if err := place(s, scratch, h, parent, pl); err != nil {
		return nil, err
	}
	cmd := commands.NewMutation(i18n.Tr("editor.history.create", "Create {name}", i18n.Str("name", t.Name)), commands.EnvFor(s),
		commands.Insert(parent, index, string(sceneio.SaveSubtree(scratch, h, true))))
	if err := d.History.Exec(cmd); err != nil {
		return nil, err
	}
	created := cmd.Created()
	ctx.Select(created...)
	return created, nil
}

func Instantiate(ctx *context.Editor, t Template, parent ids.NodeID, index int, at *mathx.Vec2) ([]ids.NodeID, error) {
	return InstantiatePlace(ctx, t, parent, index, Place{At2: at})
}

func InstantiateAt3(ctx *context.Editor, t Template, parent ids.NodeID, at mathx.Vec3) ([]ids.NodeID, error) {
	return InstantiatePlace(ctx, t, parent, -1, Place{At3: &at})
}
