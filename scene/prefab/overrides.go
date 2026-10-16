package prefab

import (
	"cmp"
	"slices"

	"aego/schema"
)

type PropOverride struct {
	Node   Key
	Prop   PropKey
	Value  schema.Value
	Orphan bool
}

type CompAdd struct {
	Node   Key
	Comp   Comp
	Orphan bool
}

type CompRemove struct {
	Node  Key
	Owner string
}

type NodeAdd struct {
	Parent Key
	Node   *Node
	Orphan bool
}

type ChildOrder struct {
	Parent   Key
	Children []Key
}

type Overrides struct {
	Props       []PropOverride
	AddComps    []CompAdd
	RemoveComps []CompRemove
	AddNodes    []NodeAdd
	Remove      []Key
	Order       []ChildOrder
}

func (o Overrides) Empty() bool {
	return len(o.Props) == 0 && len(o.AddComps) == 0 && len(o.RemoveComps) == 0 &&
		len(o.AddNodes) == 0 && len(o.Remove) == 0 && len(o.Order) == 0
}

func (o Overrides) Clone() Overrides {
	out := Overrides{
		Props:       slices.Clone(o.Props),
		RemoveComps: slices.Clone(o.RemoveComps),
		Remove:      slices.Clone(o.Remove),
	}
	for _, a := range o.AddComps {
		a.Comp = a.Comp.Clone()
		out.AddComps = append(out.AddComps, a)
	}
	for _, a := range o.AddNodes {
		a.Node = a.Node.Clone()
		out.AddNodes = append(out.AddNodes, a)
	}
	for _, ord := range o.Order {
		ord.Children = slices.Clone(ord.Children)
		out.Order = append(out.Order, ord)
	}
	return out
}

func (o Overrides) Orphans() Overrides {
	var out Overrides
	for _, p := range o.Props {
		if p.Orphan {
			out.Props = append(out.Props, p)
		}
	}
	for _, a := range o.AddComps {
		if a.Orphan {
			a.Comp = a.Comp.Clone()
			out.AddComps = append(out.AddComps, a)
		}
	}
	for _, a := range o.AddNodes {
		if a.Orphan {
			a.Node = a.Node.Clone()
			out.AddNodes = append(out.AddNodes, a)
		}
	}
	return out
}

func (o *Overrides) Merge(other Overrides) {
	c := other.Clone()
	o.Props = append(o.Props, c.Props...)
	o.AddComps = append(o.AddComps, c.AddComps...)
	o.RemoveComps = append(o.RemoveComps, c.RemoveComps...)
	o.AddNodes = append(o.AddNodes, c.AddNodes...)
	o.Remove = append(o.Remove, c.Remove...)
	o.Order = append(o.Order, c.Order...)
}

func boolOrder(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	}
	return 1
}

func (o *Overrides) Sort() {
	slices.SortStableFunc(o.Props, func(a, b PropOverride) int {
		return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.Prop.Owner, b.Prop.Owner),
			cmp.Compare(a.Prop.Field, b.Prop.Field), boolOrder(a.Orphan, b.Orphan))
	})
	slices.SortStableFunc(o.AddComps, func(a, b CompAdd) int {
		return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.Comp.Owner, b.Comp.Owner), boolOrder(a.Orphan, b.Orphan))
	})
	slices.SortStableFunc(o.RemoveComps, func(a, b CompRemove) int {
		return cmp.Or(cmp.Compare(a.Node, b.Node), cmp.Compare(a.Owner, b.Owner))
	})
	slices.SortStableFunc(o.AddNodes, func(a, b NodeAdd) int {
		return cmp.Compare(a.Parent, b.Parent)
	})
	slices.Sort(o.Remove)
	o.Remove = slices.Compact(o.Remove)
	slices.SortStableFunc(o.Order, func(a, b ChildOrder) int {
		return cmp.Compare(a.Parent, b.Parent)
	})
}
