package author

import (
	"aego/scene/prefab"
	"aego/schema"
)

type Conflict struct {
	Node   prefab.Key
	Prop   prefab.PropKey
	Before schema.Value
	After  schema.Value
	Mine   schema.Value
}

func Rebase(lib prefab.Library, o prefab.Overrides, oldP, newP *prefab.Node) (prefab.Overrides, []Conflict, error) {
	base, _, err := prefab.Expand(lib, newP, nil)
	if err != nil {
		return prefab.Overrides{}, nil, err
	}
	tree, orph, err := prefab.Expand(lib, newP, &o)
	if err != nil {
		return prefab.Overrides{}, nil, err
	}
	out := prefab.Diff(tree, base)
	out.Merge(o.Orphans())
	out.Merge(orph)
	out.Sort()
	if oldP == nil || oldP == newP {
		return out, nil, nil
	}
	before, _, err := prefab.Expand(lib, oldP, nil)
	if err != nil {
		return out, nil, err
	}
	return out, conflicts(out, before, base), nil
}

func Normalize(lib prefab.Library, p *prefab.Node, o prefab.Overrides) (prefab.Overrides, error) {
	out, _, err := Rebase(lib, o, p, p)
	return out, err
}

func Same(a, b prefab.Overrides) bool {
	return prefab.FormatOverrides(a) == prefab.FormatOverrides(b)
}

func conflicts(o prefab.Overrides, before, after *prefab.Node) []Conflict {
	bn, _ := prefab.Index(before)
	an, _ := prefab.Index(after)
	var out []Conflict
	for _, p := range o.Props {
		if p.Orphan || (p.Node == after.Key && p.Prop.Owner == prefab.OwnerTransform) {
			continue
		}
		bv, okb := bn[p.Node].Get(p.Prop)
		av, oka := an[p.Node].Get(p.Prop)
		if okb && oka && !prefab.Equal(bv, av) {
			out = append(out, Conflict{Node: p.Node, Prop: p.Prop, Before: bv, After: av, Mine: p.Value})
		}
	}
	return out
}
