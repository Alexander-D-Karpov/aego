package inspector

import (
	"strings"

	"aego/schema"
)

type Meta struct {
	DisplayName string
	Category    string
	Icon        string
	Order       int32
}

func Module(full string) string {
	if i := strings.IndexByte(full, '.'); i > 0 {
		return full[:i]
	}
	return ""
}

func (r *Registry) SetMeta(t schema.TypeID, m Meta) {
	r.meta[t] = m
	delete(r.resolved, t)
}

func RegisterMeta[T any](r *Registry, reg *schema.Registry, m Meta) error {
	td, err := typeOf[T](reg)
	if err != nil {
		return err
	}
	r.SetMeta(td.ID, m)
	return nil
}

func (r *Registry) Meta(td *schema.TypeDescriptor) Meta {
	if m, ok := r.resolved[td.ID]; ok {
		return m
	}
	m := r.meta[td.ID]
	if m.DisplayName == "" {
		name := td.FullName
		if i := strings.LastIndexByte(name, '.'); i >= 0 {
			name = name[i+1:]
		}
		m.DisplayName = humanize(name)
	}
	if m.Category == "" {
		m.Category = Module(td.FullName)
	}
	if m.Icon == "" {
		m.Icon = "cat." + strings.ToLower(m.Category)
	}
	r.resolved[td.ID] = m
	return m
}
