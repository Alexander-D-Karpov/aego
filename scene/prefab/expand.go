package prefab

import (
	"slices"
	"strconv"

	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
)

type Library interface {
	Prefab(id PrefabID) (*Node, bool)
}

type expander struct {
	lib   Library
	stack []PrefabID
}

func Expand(lib Library, p *Node, o *Overrides) (*Node, Overrides, error) {
	x := &expander{lib: lib}
	return x.expand(p, o)
}

func (x *expander) expand(p *Node, o *Overrides) (*Node, Overrides, error) {
	t, err := x.build(p)
	if err != nil {
		return nil, Overrides{}, err
	}
	if o == nil {
		return t, Overrides{}, nil
	}
	orph, err := x.apply(t, o)
	if err != nil {
		return nil, Overrides{}, err
	}
	return t, orph, nil
}

func (x *expander) build(n *Node) (*Node, error) {
	if n.Instance != nil {
		return x.instance(n)
	}
	out := &Node{Local: n.Local, Space3D: n.Space3D, Key: LocalKey(n.Local), Comps: CloneComps(n.Comps)}
	for _, c := range n.Children {
		cc, err := x.build(c)
		if err != nil {
			return nil, err
		}
		out.Children = append(out.Children, cc)
	}
	return out, nil
}

func (x *expander) instance(n *Node) (*Node, error) {
	inst := n.Instance
	at := LocalKey(n.Local)
	var q *Node
	if x.lib != nil {
		q, _ = x.lib.Prefab(inst.Prefab)
	}
	if q == nil {
		comps := CloneComps(n.Comps)
		if len(comps) == 0 {
			comps = NewNode(n.Local, "Missing prefab", n.Space3D).Comps
		}
		return &Node{Local: n.Local, Space3D: n.Space3D, Key: at, Comps: comps, Missing: true, Origin: inst}, nil
	}
	if slices.Contains(x.stack, inst.Prefab) {
		return nil, errs.New(errs.PrefabCycle, "prefab contains an instance of itself", log.F("prefab", inst.Prefab.String()))
	}
	if len(x.stack) >= MaxDepth {
		return nil, errs.New(errs.PrefabDepth, "prefabs are nested deeper than "+strconv.Itoa(MaxDepth)+" levels",
			log.F("prefab", inst.Prefab.String()))
	}
	x.stack = append(x.stack, inst.Prefab)
	sub, _, err := x.expand(q, &inst.Overrides)
	x.stack = x.stack[:len(x.stack)-1]
	if err != nil {
		return nil, err
	}
	root := sub.Key
	m := map[ids.NodeID]ids.NodeID{}
	sub.Walk(func(c, _ *Node) {
		k := Rekey(c.Key, root, at)
		m[c.Key.ID()] = k.ID()
		c.Key, c.Local = k, k.ID()
	})
	sub.MapRefs(m)
	sub.Origin = inst
	return sub, nil
}

func (x *expander) apply(t *Node, o *Overrides) (Overrides, error) {
	nodes, parents := Index(t)
	for _, k := range o.Remove {
		if pk, ok := parents[k]; ok {
			nodes[pk].removeChild(k)
		}
	}
	if len(o.Remove) > 0 {
		nodes, _ = Index(t)
	}
	for _, r := range o.RemoveComps {
		if n := nodes[r.Node]; n != nil {
			if c := n.Comp(r.Owner); c != nil && c.Kind.Removable() {
				n.RemoveComp(r.Owner)
			}
		}
	}
	var orph Overrides
	for _, a := range o.AddComps {
		if a.Orphan {
			continue
		}
		n := nodes[a.Node]
		if n == nil {
			a.Comp, a.Orphan = a.Comp.Clone(), true
			orph.AddComps = append(orph.AddComps, a)
			continue
		}
		if c := n.Comp(a.Comp.Owner); c != nil {
			for _, f := range a.Comp.Fields {
				c.Set(f.Name, f.Value)
			}
			continue
		}
		n.Comps = append(n.Comps, a.Comp.Clone())
	}
	for _, p := range o.Props {
		if p.Orphan {
			continue
		}
		var c *Comp
		if n := nodes[p.Node]; n != nil {
			c = n.Comp(p.Prop.Owner)
		}
		if c == nil {
			p.Orphan = true
			orph.Props = append(orph.Props, p)
			continue
		}
		c.Set(p.Prop.Field, p.Value)
	}
	for _, a := range o.AddNodes {
		if a.Orphan {
			continue
		}
		par := nodes[a.Parent]
		if par == nil {
			a.Node, a.Orphan = a.Node.Clone(), true
			orph.AddNodes = append(orph.AddNodes, a)
			continue
		}
		sub, err := x.build(a.Node)
		if err != nil {
			return orph, err
		}
		par.Children = append(par.Children, sub)
	}
	if len(o.Order) > 0 {
		nodes, _ = Index(t)
		for _, ord := range o.Order {
			if par := nodes[ord.Parent]; par != nil {
				par.reorder(ord.Children)
			}
		}
	}
	return orph, nil
}
