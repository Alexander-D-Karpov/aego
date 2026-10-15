package inspector

import (
	"sort"

	"aego/editor/ext/inspect"
	"aego/schema"
)

type Section struct {
	Category string
	Types    []schema.TypeID
	rank     int
	pos      int
}

func groupComponents(r *inspect.Registry, reg *schema.Registry, types []schema.TypeID) []Section {
	order := map[schema.TypeID]int{}
	for i, td := range reg.Components() {
		order[td.ID] = i
	}
	var out []Section
	index := map[string]int{}
	for _, t := range types {
		td, ok := reg.ByID(t)
		if !ok {
			continue
		}
		pos, ok := order[t]
		if !ok {
			pos = len(order)
		}
		cat := r.Meta(td).Category
		rank := r.Rank(td)
		i, ok := index[cat]
		if !ok {
			i = len(out)
			index[cat] = i
			out = append(out, Section{Category: cat, rank: rank, pos: pos})
		}
		out[i].rank = min(out[i].rank, rank)
		out[i].pos = min(out[i].pos, pos)
		out[i].Types = append(out[i].Types, t)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].rank != out[b].rank {
			return out[a].rank < out[b].rank
		}
		return out[a].pos < out[b].pos
	})
	return out
}

func Sections(r *inspect.Registry, reg *schema.Registry, types []schema.TypeID) []Section {
	return groupComponents(r, reg, types)
}
