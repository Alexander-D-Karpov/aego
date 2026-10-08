package templates

import (
	"fmt"
	"sort"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/context"
	"aego/editor/history/commands"
	"aego/scene"
	"aego/scene/components"
	"aego/scene/sceneio"
)

const PixelsPerUnit = 32

type Template struct {
	ID       string
	Name     string
	Module   string
	Category string
	Space    scene.Space
	Build    func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle
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

type Registry struct {
	list []Template
}

func New() *Registry {
	return &Registry{}
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

func isRoot(t Template) bool {
	return t.Module == "" || t.Module == "engine"
}

func (r *Registry) Root() []Template {
	var out []Template
	for _, t := range r.list {
		if isRoot(t) {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) Modules() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range r.list {
		if !isRoot(t) && !seen[t.Module] {
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
		if t.Module == m {
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

func RegisterBuiltin(r *Registry) error {
	list := append([]Template{Empty(), Sprite(), Camera()}, Builtin3D()...)
	for _, t := range list {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

func Empty() Template {
	return Template{ID: "engine.empty", Name: "Empty", Module: "engine", Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
		return s.Create(parent, "Node")
	}}
}

func Sprite() Template {
	return SpriteWith(ids.AssetID{}, "Sprite", mathx.Vec2{X: 1, Y: 1})
}

func SpriteWith(id ids.AssetID, name string, size mathx.Vec2) Template {
	if name == "" {
		name = "Sprite"
	}
	return Template{ID: "engine.sprite", Name: "Sprite", Module: "engine", Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
		h := s.Create(parent, name)
		sr := scene.Add[components.SpriteRenderer](s, h)
		*sr = components.DefaultSpriteRenderer()
		sr.Sprite.SetAssetRefID(id)
		sr.Size = size
		return h
	}}
}

func Camera() Template {
	return Template{ID: "engine.camera", Name: "Camera", Module: "engine", Build: func(s *scene.Scene, parent scene.NodeHandle) scene.NodeHandle {
		h := s.Create(parent, "Camera")
		*scene.Add[components.Camera2D](s, h) = components.DefaultCamera2D()
		return h
	}}
}

func SpriteSize(ctx *context.Editor, id ids.AssetID) mathx.Vec2 {
	if ctx.Loader != nil && !id.IsZero() {
		if reg, ok := ctx.Loader.Texture(id); ok && reg.W > 0 && reg.H > 0 {
			return mathx.Vec2{X: float32(reg.W) / PixelsPerUnit, Y: float32(reg.H) / PixelsPerUnit}
		}
	}
	return mathx.Vec2{X: 1, Y: 1}
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
	if err := place(s, scratch, h, parent, pl); err != nil {
		return nil, err
	}
	cmd := &commands.Create{
		Parent:   parent,
		Index:    index,
		Fragment: sceneio.SaveSubtree(scratch, h, true),
		Name:     t.Name,
	}
	if err := d.History.Exec(cmd); err != nil {
		return nil, err
	}
	created := append([]ids.NodeID(nil), cmd.Roots()...)
	ctx.Select(created...)
	return created, nil
}

func Instantiate(ctx *context.Editor, t Template, parent ids.NodeID, index int, at *mathx.Vec2) ([]ids.NodeID, error) {
	return InstantiatePlace(ctx, t, parent, index, Place{At2: at})
}

func InstantiateAt3(ctx *context.Editor, t Template, parent ids.NodeID, at mathx.Vec3) ([]ids.NodeID, error) {
	return InstantiatePlace(ctx, t, parent, -1, Place{At3: &at})
}
