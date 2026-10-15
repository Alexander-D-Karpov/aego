package inspect

import (
	"strings"
	"unicode"

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
		m.DisplayName = Humanize(name)
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

func Humanize(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i, r := range rs {
		if r == '_' {
			b.WriteByte(' ')
			continue
		}
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		prev := rs[i-1]
		var next rune
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		if unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
