package schema

import "strings"

// ShortName is the part of a full name after its last dot.
func ShortName(full string) string {
	return full[strings.LastIndexByte(full, '.')+1:]
}

// ComponentNamed resolves a short component name (D93): a unique project component wins over engine modules; otherwise the name must be unique.
func (r *Registry) ComponentNamed(short string) (*TypeDescriptor, bool) {
	var project, builtin *TypeDescriptor
	np, nb := 0, 0
	for _, td := range r.Components() {
		if ShortName(td.FullName) != short {
			continue
		}
		if td.Builtin {
			builtin, nb = td, nb+1
		} else {
			project, np = td, np+1
		}
	}
	switch {
	case np == 1:
		return project, true
	case np == 0 && nb == 1:
		return builtin, true
	}
	return nil, false
}

// PreferProject drops engine-module types from a list of candidates that also holds project types (D93).
func PreferProject(found []*TypeDescriptor) []*TypeDescriptor {
	n := 0
	for _, td := range found {
		if !td.Builtin {
			n++
		}
	}
	if n == 0 || n == len(found) {
		return found
	}
	out := make([]*TypeDescriptor, 0, n)
	for _, td := range found {
		if !td.Builtin {
			out = append(out, td)
		}
	}
	return out
}
