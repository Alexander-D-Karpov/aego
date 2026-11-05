package inspect

import (
	"errors"
	"fmt"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/editor/core/context"
	"aego/editor/core/history/commands"
	"aego/schema"
)

type Custom interface {
	Draw(ctx *context.Editor, nodes []ids.NodeID, defaultFields func())
}

// FieldVisibility hides inapplicable fields only in presentation, preserving document properties, typed controls, multiselection, animation bindings and History.
type FieldVisibility interface {
	FieldVisible(ctx *context.Editor, nodes []ids.NodeID, field *schema.FieldDescriptor) bool
}

// FieldValidation supplies a localized explanation for an invalid field.
type FieldValidation interface {
	FieldError(ctx *context.Editor, nodes []ids.NodeID, field *schema.FieldDescriptor) string
}

// NormalizedCurve marks curves whose key times must stay within 0..1.
type NormalizedCurve interface {
	NormalizedCurve(ctx *context.Editor, nodes []ids.NodeID, field *schema.FieldDescriptor) bool
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
	order    map[string]int
	editor   *context.Editor
}

func (r *Registry) SetEditor(ed *context.Editor) {
	r.editor = ed
}

func (r *Registry) DrawCustom(c Custom, nodes []ids.NodeID, def func()) {
	c.Draw(r.editor, nodes, def)
}

func (r *Registry) FieldVisible(t schema.TypeID, nodes []ids.NodeID, field *schema.FieldDescriptor) bool {
	c, ok := r.Custom(t)
	if !ok {
		return true
	}
	filter, ok := c.(FieldVisibility)
	return !ok || filter.FieldVisible(r.editor, nodes, field)
}

func (r *Registry) FieldError(t schema.TypeID, nodes []ids.NodeID, field *schema.FieldDescriptor) string {
	c, ok := r.Custom(t)
	if !ok {
		return ""
	}
	if validation, ok := c.(FieldValidation); ok {
		return validation.FieldError(r.editor, nodes, field)
	}
	return ""
}

func (r *Registry) NormalizedCurve(t schema.TypeID, nodes []ids.NodeID, field *schema.FieldDescriptor) bool {
	c, ok := r.Custom(t)
	if !ok {
		return false
	}
	if limits, ok := c.(NormalizedCurve); ok {
		return limits.NormalizedCurve(r.editor, nodes, field)
	}
	return false
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

func (r *Registry) SetModules(ids []string) {
	r.order = make(map[string]int, len(ids))
	for i, id := range ids {
		r.order[id] = i
	}
}

func (r *Registry) Rank(td *schema.TypeDescriptor) int {
	if i, ok := r.order[td.Owner]; ok {
		return i
	}
	return len(r.order)
}
