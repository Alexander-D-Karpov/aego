package inspector

import (
	"strings"

	"aego/schema"
)

type Meta struct {
	DisplayName string
	Category    string
	Icon        string
	Badge       string
}

func Module(full string) string {
	if i := strings.IndexByte(full, '.'); i > 0 {
		return full[:i]
	}
	return ""
}

func (r *Registry) Meta(td *schema.TypeDescriptor) Meta {
	if m, ok := r.resolved[td.ID]; ok {
		return m
	}
	m := Meta{DisplayName: td.DisplayName, Category: td.Category, Icon: td.Icon}
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
	if !td.Builtin {
		m.Badge = td.Owner
		if m.Badge == "" {
			m.Badge = Module(td.FullName)
		}
	}
	r.resolved[td.ID] = m
	return m
}
