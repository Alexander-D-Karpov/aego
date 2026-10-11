package inspector

import (
	"errors"
	"fmt"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/context"
	"aego/editor/history/commands"
	"aego/schema"
)

type Custom interface {
	Draw(ctx *context.Editor, nodes []ids.NodeID, defaultFields func())
}

type CustomFunc func(ctx *context.Editor, nodes []ids.NodeID, defaultFields func())

func (f CustomFunc) Draw(ctx *context.Editor, nodes []ids.NodeID, defaultFields func()) {
	f(ctx, nodes, defaultFields)
}

func AsCustom(v any) (Custom, bool) {
	switch c := v.(type) {
	case Custom:
		return c, true
	case func(*context.Editor, []ids.NodeID, func()):
		return CustomFunc(c), true
	}
	return nil, false
}

type Registry struct {
	schema   *schema.Registry
	custom   map[schema.TypeID]Custom
	disabled map[schema.TypeID]string
	resolved map[schema.TypeID]Meta
}

func NewRegistry() *Registry {
	return &Registry{
		custom:   map[schema.TypeID]Custom{},
		disabled: map[schema.TypeID]string{},
		resolved: map[schema.TypeID]Meta{},
	}
}

func (r *Registry) Load(sr *schema.Registry) error {
	r.schema = sr
	clear(r.resolved)
	var all []error
	for _, td := range sr.Components() {
		if td.Inspector == nil {
			continue
		}
		c, ok := AsCustom(td.Inspector)
		if !ok {
			all = append(all, errs.New(errs.EditorPanel,
				"custom inspector of "+td.FullName+" has the unsupported type "+fmt.Sprintf("%T", td.Inspector),
				log.F("module", td.Owner)))
			continue
		}
		r.custom[td.ID] = c
	}
	return errors.Join(all...)
}

func (r *Registry) RegisterCustomID(t schema.TypeID, c Custom) error {
	if _, dup := r.custom[t]; dup {
		return errs.New(errs.EditorPanel, "custom inspector is already registered")
	}
	r.custom[t] = c
	delete(r.disabled, t)
	return nil
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

func DefaultValues(td *schema.TypeDescriptor) []commands.FieldValue {
	if td.Default == nil {
		return nil
	}
	p := td.Default()
	out := make([]commands.FieldValue, 0, len(td.Fields))
	for i := range td.Fields {
		var v schema.Value
		td.Fields[i].Get(p, &v)
		out = append(out, commands.FieldValue{Field: td.Fields[i].ID, Value: v})
	}
	return out
}

func (r *Registry) Defaults(t schema.TypeID) []commands.FieldValue {
	if r.schema == nil {
		return nil
	}
	td, ok := r.schema.ByID(t)
	if !ok {
		return nil
	}
	return DefaultValues(td)
}
