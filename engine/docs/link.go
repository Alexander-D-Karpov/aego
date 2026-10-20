//go:build !aego_runtime

package docs

import "strings"

func aliasesOf(e *Entry) []string {
	id := e.ID
	var out []string
	if i := strings.LastIndexByte(id, '/'); i >= 0 && i+1 < len(id) {
		out = append(out, id[i+1:])
	}
	switch e.Kind {
	case Type, Field, Method, Function:
		parts := strings.Split(id, ".")
		if len(parts) >= 2 {
			out = append(out, strings.Join(parts[1:], "."))
		}
		if e.Kind == Type && len(parts) >= 2 {
			out = append(out, parts[len(parts)-1])
		}
	}
	return out
}

func (r *Registry) Aliases() map[string]string {
	if r.alias != nil {
		return r.alias
	}
	count := map[string]int{}
	first := map[string]string{}
	for i := range r.entries {
		e := &r.entries[i]
		for _, a := range aliasesOf(e) {
			if a == "" || a == e.ID {
				continue
			}
			if _, exact := r.byID[a]; exact {
				continue
			}
			if first[a] != e.ID {
				count[a]++
			}
			if _, ok := first[a]; !ok {
				first[a] = e.ID
			}
		}
	}
	r.alias = map[string]string{}
	for a, n := range count {
		if n == 1 {
			r.alias[a] = first[a]
		}
	}
	return r.alias
}

func (r *Registry) Link(text string) (Entry, bool) {
	text = strings.TrimSuffix(strings.TrimSpace(text), "()")
	if text == "" || strings.ContainsAny(text, " \t\n(") {
		return Entry{}, false
	}
	if e, ok := r.Resolve(text); ok {
		return e, true
	}
	if id, ok := r.Aliases()[text]; ok {
		return r.Get(id)
	}
	return Entry{}, false
}
