package module

import (
	"sort"

	"aego/scene"
	"aego/script/bind"
)

func Audit(types *scene.Types, script *bind.Registry, mods ...Module) (*IconReport, error) {
	rep := &IconReport{used: map[string]bool{}, seen: map[string]bool{}}
	r := New(types, script)
	r.audit = rep
	for _, m := range mods {
		if err := r.Add(m); err != nil {
			return rep, err
		}
	}
	if _, err := r.Build(); err != nil {
		return rep, err
	}
	for _, set := range r.icons {
		for _, id := range set.IDs {
			if !rep.used[id] {
				rep.Problems = append(rep.Problems, IconProblem{Kind: IconUnused, Owner: set.Owner, Icon: id})
			}
		}
	}
	sort.SliceStable(rep.Problems, func(i, j int) bool {
		a, b := rep.Problems[i], rep.Problems[j]
		if a.Owner != b.Owner {
			return a.Owner < b.Owner
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Icon < b.Icon
	})
	return rep, nil
}
