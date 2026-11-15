//go:build !aego_runtime

package docs

import (
	"regexp"
	"strings"
)

var parameterName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// documentParams changes documentation labels only, never bindings or ABI.
func (r *Registry) documentParams(e *Entry, names []string) {
	if (e.Kind != Method && e.Kind != Function) || len(names) != len(e.Params) {
		r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "params must name every parameter of an existing method or function"})
		return
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !parameterName.MatchString(name) || seen[name] {
			r.problems = append(r.problems, Problem{ID: e.ID, Error: true, Message: "params contains an invalid or duplicate name: " + name})
			return
		}
		seen[name] = true
	}
	// Replace simultaneously so swapping two labels cannot change both to one.
	oldNames := map[string]string{}
	for i, name := range names {
		oldNames[e.Params[i].Name] = name
		e.Params[i].Name = name
	}
	e.Signature = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`).ReplaceAllStringFunc(e.Signature, func(word string) string {
		if name, ok := oldNames[word]; ok {
			return name
		}
		return word
	})
}

// publicType drops ABI serialization markers while retaining the qualified
// public type ID. Reference rendering can link this ID to its field list.
func publicType(raw string) string {
	for _, prefix := range []string{"struct:", "enum:", "handle:"} {
		raw = strings.ReplaceAll(raw, prefix, "")
	}
	return raw
}
