package prefab

import "slices"

type differ struct {
	o    Overrides
	base map[Key]*Node
	bp   map[Key]Key
	inst map[Key]*Node
	ip   map[Key]Key
}

func Diff(inst, base *Node) Overrides {
	d := &differ{}
	d.base, d.bp = Index(base)
	d.inst, d.ip = Index(inst)
	base.Walk(func(n, parent *Node) {
		if parent == nil {
			return
		}
		if _, ok := d.inst[n.Key]; ok {
			return
		}
		if _, ok := d.inst[parent.Key]; ok {
			d.o.Remove = append(d.o.Remove, n.Key)
		}
	})
	if _, ok := d.base[inst.Key]; ok {
		d.node(inst, true)
	}
	d.o.Sort()
	return d.o
}

func (d *differ) node(n *Node, root bool) {
	b := d.base[n.Key]
	d.comps(n, b, root)
	var expected, actual []Key
	for _, c := range b.Children {
		if d.ip[c.Key] == n.Key {
			expected = append(expected, c.Key)
		}
	}
	var known []*Node
	for _, c := range n.Children {
		if _, ok := d.base[c.Key]; !ok {
			d.o.AddNodes = append(d.o.AddNodes, NodeAdd{Parent: n.Key, Node: collapse(c)})
			expected = append(expected, c.Key)
			actual = append(actual, c.Key)
			continue
		}
		if d.bp[c.Key] == n.Key {
			actual = append(actual, c.Key)
			known = append(known, c)
		}
	}
	if !slices.Equal(expected, actual) {
		d.o.Order = append(d.o.Order, ChildOrder{Parent: n.Key, Children: slices.Clone(actual)})
	}
	for _, c := range known {
		d.node(c, false)
	}
}

func (d *differ) comps(n, b *Node, root bool) {
	for i := range b.Comps {
		bc := &b.Comps[i]
		if bc.Kind.Removable() && n.Comp(bc.Owner) == nil {
			d.o.RemoveComps = append(d.o.RemoveComps, CompRemove{Node: n.Key, Owner: bc.Owner})
		}
	}
	for i := range n.Comps {
		ic := &n.Comps[i]
		bc := b.Comp(ic.Owner)
		if bc == nil {
			d.o.AddComps = append(d.o.AddComps, CompAdd{Node: n.Key, Comp: ic.Clone()})
			continue
		}
		force := root && ic.Kind == CompTransform
		for _, f := range ic.Fields {
			bv, ok := bc.Get(f.Name)
			if force || !ok || !Equal(bv, f.Value) {
				d.o.Props = append(d.o.Props, PropOverride{Node: n.Key, Prop: PropKey{Owner: ic.Owner, Field: f.Name}, Value: f.Value})
			}
		}
	}
}

func collapse(n *Node) *Node {
	if n.Origin != nil {
		out := &Node{Local: n.Local, Space3D: n.Space3D, Instance: n.Origin.Clone()}
		if n.Missing {
			out.Comps = CloneComps(n.Comps)
		}
		return out
	}
	out := &Node{Local: n.Local, Space3D: n.Space3D, Comps: CloneComps(n.Comps)}
	for _, c := range n.Children {
		out.Children = append(out.Children, collapse(c))
	}
	return out
}
