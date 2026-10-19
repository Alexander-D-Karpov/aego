package module

import (
	"sort"

	"aego/scene"
	"aego/script/bind"
)

func Audit(types *scene.Types, script *bind.Registry, mods ...Module) (*IconReport, error) {
	return AuditUses(nil, types, script, mods...)
}

func AuditUses(extra []IconUse, types *scene.Types, script *bind.Registry, mods ...Module) (*IconReport, error) {
	rep := &IconReport{used: map[string]bool{}, seen: map[string]bool{}, owners: map[string]string{}}
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
	for _, u := range extra {
		rep.Uses = append(rep.Uses, u)
		rep.used[u.ID] = true
		if _, ok := rep.owners[u.ID]; !ok {
			rep.Problems = append(rep.Problems, IconProblem{Kind: IconMissing, Owner: u.Owner, Icon: u.ID, By: u.By})
		}
	}
	for _, set := range r.icons {
		for _, id := range set.IDs {
			if !rep.used[id] {
				rep.Problems = append(rep.Problems, IconProblem{Kind: IconUnused, Owner: set.Owner, Icon: id})
			}
		}
	}
	if extra != nil {
		for id, owner := range rep.owners {
			if owner == "engine" && !rep.used[id] {
				rep.Problems = append(rep.Problems, IconProblem{Kind: IconUnused, Owner: owner, Icon: id})
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
