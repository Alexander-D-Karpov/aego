package inspector

import (
	"sort"
	"strings"

	"aego/schema"
)

type Section struct {
	Category string
	Types    []schema.TypeID
}

func categoryRank(c string) int {
	order := [...]string{"Rendering", "Camera", "Scripts", "Physics", "Audio"}
	for i, o := range order {
		if strings.EqualFold(o, c) {
			return i
		}
	}
	return len(order)
}

func groupComponents(r *Registry, reg *schema.Registry, types []schema.TypeID) []Section {
	var out []Section
	index := map[string]int{}
	for _, t := range types {
		td, ok := reg.ByID(t)
		if !ok {
			continue
		}
		cat := r.Meta(td).Category
		i, ok := index[cat]
		if !ok {
			i = len(out)
			index[cat] = i
			out = append(out, Section{Category: cat})
		}
		out[i].Types = append(out[i].Types, t)
	}
	sort.SliceStable(out, func(a, b int) bool {
		ra, rb := categoryRank(out[a].Category), categoryRank(out[b].Category)
		if ra != rb {
			return ra < rb
		}
		return strings.ToLower(out[a].Category) < strings.ToLower(out[b].Category)
	})
	return out
}
