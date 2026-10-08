package inspector

import (
	"reflect"
	"unsafe"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/context"
	"aego/editor/history/commands"
	"aego/scene/components"
	"aego/schema"
)

type Custom interface {
	Draw(ctx *context.Editor, nodes []ids.NodeID, defaultFields func())
}

type CustomFunc func(ctx *context.Editor, nodes []ids.NodeID, defaultFields func())

func (f CustomFunc) Draw(ctx *context.Editor, nodes []ids.NodeID, defaultFields func()) {
	f(ctx, nodes, defaultFields)
}

type Registry struct {
	custom   map[schema.TypeID]Custom
	defaults map[schema.TypeID]func() []commands.FieldValue
	disabled map[schema.TypeID]string
	meta     map[schema.TypeID]Meta
	resolved map[schema.TypeID]Meta
}

func NewRegistry() *Registry {
	return &Registry{
		custom:   map[schema.TypeID]Custom{},
		defaults: map[schema.TypeID]func() []commands.FieldValue{},
		disabled: map[schema.TypeID]string{},
		meta:     map[schema.TypeID]Meta{},
		resolved: map[schema.TypeID]Meta{},
	}
}

func typeOf[T any](reg *schema.Registry) (*schema.TypeDescriptor, error) {
	rt := reflect.TypeFor[T]()
	td, ok := reg.ByGoType(rt)
	if !ok {
		return nil, errs.New(errs.SchemaUnknownType, "type is not registered", log.F("type", rt.String()))
	}
	return td, nil
}

func RegisterCustom[T any](r *Registry, reg *schema.Registry, c Custom) error {
	td, err := typeOf[T](reg)
	if err != nil {
		return err
	}
	if _, dup := r.custom[td.ID]; dup {
		return errs.New(errs.EditorPanel, "custom inspector is already registered", log.F("type", td.FullName))
	}
	r.custom[td.ID] = c
	return nil
}

func (r *Registry) RegisterCustomID(t schema.TypeID, c Custom) error {
	if _, dup := r.custom[t]; dup {
		return errs.New(errs.EditorPanel, "custom inspector is already registered")
	}
	r.custom[t] = c
	delete(r.disabled, t)
	return nil
}

func RegisterDefaults[T any](r *Registry, reg *schema.Registry, fn func() T) error {
	td, err := typeOf[T](reg)
	if err != nil {
		return err
	}
	r.defaults[td.ID] = func() []commands.FieldValue {
		v := fn()
		p := unsafe.Pointer(&v)
		out := make([]commands.FieldValue, 0, len(td.Fields))
		for i := range td.Fields {
			var val schema.Value
			td.Fields[i].Get(p, &val)
			out = append(out, commands.FieldValue{Field: td.Fields[i].ID, Value: val})
		}
		return out
	}
	return nil
}

func RegisterBuiltin(r *Registry, reg *schema.Registry) error {
	if err := RegisterDefaults(r, reg, components.DefaultSpriteRenderer); err != nil {
		return err
	}
	if err := RegisterDefaults(r, reg, components.DefaultCamera2D); err != nil {
		return err
	}
	if err := RegisterDefaults(r, reg, components.DefaultCamera3D); err != nil {
		return err
	}
	if err := RegisterDefaults(r, reg, components.DefaultMeshRenderer); err != nil {
		return err
	}
	if err := RegisterMeta[components.SpriteRenderer](r, reg, Meta{DisplayName: "Sprite Renderer", Category: "Rendering", Icon: "cat.rendering"}); err != nil {
		return err
	}
	if err := RegisterMeta[components.MeshRenderer](r, reg, Meta{DisplayName: "Mesh Renderer", Category: "Rendering", Icon: "cat.rendering"}); err != nil {
		return err
	}
	if err := RegisterMeta[components.Camera2D](r, reg, Meta{DisplayName: "Camera 2D", Category: "Camera", Icon: "cat.camera"}); err != nil {
		return err
	}
	if err := RegisterMeta[components.ScriptComponent](r, reg, Meta{DisplayName: "Scripts", Category: "Scripts", Icon: "cat.scripts"}); err != nil {
		return err
	}
	return RegisterMeta[components.Camera3D](r, reg, Meta{DisplayName: "Camera 3D", Category: "Camera", Icon: "cat.camera"})
}

func (r *Registry) Custom(t schema.TypeID) (Custom, bool) {
	if _, off := r.disabled[t]; off {
		return nil, false
	}
	c, ok := r.custom[t]
	return c, ok
}

func (r *Registry) Disable(t schema.TypeID, reason string) {
	r.disabled[t] = reason
}

func (r *Registry) Defaults(t schema.TypeID) []commands.FieldValue {
	if fn, ok := r.defaults[t]; ok {
		return fn()
	}
	return nil
}
