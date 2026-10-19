package prefabs

import (
	"slices"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/scene/prefab"
	"aego/scene/prefab/author"
	"aego/scene/sceneio"
	"aego/schema"
)

type baker struct {
	lib    *sceneio.Library
	edits  map[ids.AssetID]*prefab.Node
	nested map[*prefab.Node]ids.AssetID
}

type dest struct {
	node   *prefab.Node
	parent *prefab.Node
	inst   *prefab.Node
	added  *prefab.Node
	owner  ids.AssetID
	rel    prefab.Key
	maps   []map[ids.NodeID]ids.NodeID
}

func newBaker(lib *sceneio.Library) *baker {
	return &baker{lib: lib, edits: map[ids.AssetID]*prefab.Node{}, nested: map[*prefab.Node]ids.AssetID{}}
}

func (b *baker) model(id ids.AssetID) (*prefab.Node, error) {
	if m, ok := b.edits[id]; ok {
		return m, nil
	}
	p, ok := b.lib.Prefab(id)
	if !ok {
		return nil, errs.New(errs.EditorCommand, "prefab "+id.Short()+" is not loaded")
	}
	m := p.Clone()
	b.edits[id] = m
	return m, nil
}

func findLocal(m *prefab.Node, local ids.NodeID) (*prefab.Node, *prefab.Node) {
	var found, parent *prefab.Node
	m.Walk(func(n, p *prefab.Node) {
		if found == nil && n.Local == local {
			found, parent = n, p
		}
	})
	return found, parent
}

func (b *baker) remap(q *prefab.Node, at prefab.Key) (map[ids.NodeID]ids.NodeID, error) {
	tree, _, err := prefab.Expand(b.lib, q, nil)
	if err != nil {
		return nil, err
	}
	out := map[ids.NodeID]ids.NodeID{}
	tree.Walk(func(n, _ *prefab.Node) {
		out[prefab.Rekey(n.Key, tree.Key, at).ID()] = n.Key.ID()
	})
	return out, nil
}

func (b *baker) route(id ids.AssetID, key prefab.Key, nearest, self bool) (dest, error) {
	m, err := b.model(id)
	if err != nil {
		return dest{}, err
	}
	head, rest, nested := strings.Cut(string(key), "/")
	local, perr := ids.ParseNodeID(head)
	if perr != nil {
		return dest{}, errs.New(errs.EditorCommand, "bad prefab key "+string(key))
	}
	n, parent := findLocal(m, local)
	if n == nil {
		return dest{}, errs.New(errs.EditorCommand, "this node cannot be applied, it is not stored in the prefab")
	}
	if n.Instance == nil || (self && !nested) {
		if nested {
			return dest{}, errs.New(errs.EditorCommand, "bad prefab key "+string(key))
		}
		return dest{node: n, parent: parent, owner: id}, nil
	}
	qid := n.Instance.Prefab
	q, ok := b.lib.Prefab(qid)
	if !ok {
		return dest{}, errs.New(errs.EditorCommand, "nested prefab "+qid.Short()+" is not loaded")
	}
	rel := prefab.LocalKey(q.Local)
	if nested {
		rel = prefab.Key(rest)
	}
	if nearest && nested {
		if a, ap, ok := addedIn(n, rel); ok {
			if a.Instance != nil {
				return dest{}, errs.New(errs.EditorCommand, "a prefab added inside a nested prefab cannot be applied to the nearest prefab yet, apply it to the outer prefab")
			}
			d := dest{node: a, parent: ap, owner: id}
			if ap == nil {
				d.added = n
			}
			return d, nil
		}
	}
	mp, err := b.remap(q, prefab.LocalKey(n.Local))
	if err != nil {
		return dest{}, err
	}
	if nearest {
		d, err := b.route(qid, rel, true, self)
		if err != nil {
			return d, err
		}
		d.maps = append([]map[ids.NodeID]ids.NodeID{mp}, d.maps...)
		return d, nil
	}
	return dest{inst: n, rel: rel, owner: id, maps: []map[ids.NodeID]ids.NodeID{mp}}, nil
}

func (d dest) value(v schema.Value) schema.Value {
	for _, m := range d.maps {
		v, _ = prefab.MapValue(v, m)
	}
	return v
}

func (d dest) comp(c prefab.Comp) prefab.Comp {
	c = c.Clone()
	for i := range c.Fields {
		c.Fields[i].Value = d.value(c.Fields[i].Value)
	}
	return c
}

func (d dest) tree(n *prefab.Node) *prefab.Node {
	n = n.Clone()
	for _, m := range d.maps {
		n.MapRefs(m)
	}
	return n
}

func (b *baker) push(d dest, fn func(*prefab.Overrides)) {
	fn(&d.inst.Instance.Overrides)
	b.nested[d.inst] = d.inst.Instance.Prefab
}

func (b *baker) bake(id ids.AssetID, o prefab.Overrides, nearest bool) error {
	for _, a := range o.AddComps {
		d, err := b.route(id, a.Node, nearest, false)
		if err != nil {
			return err
		}
		c := d.comp(a.Comp)
		if d.inst != nil {
			b.push(d, func(x *prefab.Overrides) { x.AddComps = append(x.AddComps, prefab.CompAdd{Node: d.rel, Comp: c}) })
			continue
		}
		if ex := d.node.Comp(c.Owner); ex != nil {
			for _, f := range c.Fields {
				ex.Set(f.Name, f.Value)
			}
			continue
		}
		d.node.Comps = append(d.node.Comps, c)
	}
	for _, p := range o.Props {
		d, err := b.route(id, p.Node, nearest, false)
		if err != nil {
			return err
		}
		v := d.value(p.Value)
		if d.inst != nil {
			b.push(d, func(x *prefab.Overrides) {
				x.Props = append(x.Props, prefab.PropOverride{Node: d.rel, Prop: p.Prop, Value: v})
			})
			continue
		}
		c := d.node.Comp(p.Prop.Owner)
		if c == nil {
			return errs.New(errs.EditorCommand, "component "+p.Prop.Owner+" is not in the prefab, apply the component first")
		}
		c.Set(p.Prop.Field, v)
	}
	for _, r := range o.RemoveComps {
		d, err := b.route(id, r.Node, nearest, false)
		if err != nil {
			return err
		}
		if d.inst != nil {
			b.push(d, func(x *prefab.Overrides) {
				x.RemoveComps = append(x.RemoveComps, prefab.CompRemove{Node: d.rel, Owner: r.Owner})
			})
			continue
		}
		d.node.RemoveComp(r.Owner)
	}
	for _, a := range o.AddNodes {
		d, err := b.route(id, a.Parent, nearest, false)
		if err != nil {
			return err
		}
		n := d.tree(a.Node)
		if d.inst != nil {
			b.push(d, func(x *prefab.Overrides) { x.AddNodes = append(x.AddNodes, prefab.NodeAdd{Parent: d.rel, Node: n}) })
			continue
		}
		d.node.Children = append(d.node.Children, n)
	}
	for _, k := range o.Remove {
		d, err := b.route(id, k, nearest, true)
		if err != nil {
			return err
		}
		if d.inst != nil {
			b.push(d, func(x *prefab.Overrides) { x.Remove = append(x.Remove, d.rel) })
			continue
		}
		target := d.node
		if d.parent == nil {
			if d.added == nil {
				return errs.New(errs.EditorCommand, "the prefab root cannot be removed")
			}
			ov := &d.added.Instance.Overrides
			ov.AddNodes = slices.DeleteFunc(ov.AddNodes, func(a prefab.NodeAdd) bool { return a.Node == target })
			continue
		}
		d.parent.Children = slices.DeleteFunc(d.parent.Children, func(c *prefab.Node) bool { return c == target })
	}
	for _, ord := range o.Order {
		d, err := b.route(id, ord.Parent, nearest, false)
		if err != nil {
			return err
		}
		if d.inst != nil {
			var rel []prefab.Key
			for _, ck := range ord.Children {
				cd, err := b.route(id, ck, nearest, true)
				if err != nil || cd.inst == nil {
					continue
				}
				rel = append(rel, cd.rel)
			}
			b.push(d, func(x *prefab.Overrides) { x.Order = append(x.Order, prefab.ChildOrder{Parent: d.rel, Children: rel}) })
			continue
		}
		var out []*prefab.Node
		for _, ck := range ord.Children {
			cd, err := b.route(id, ck, nearest, true)
			if err != nil || cd.node == nil || !slices.Contains(d.node.Children, cd.node) || slices.Contains(out, cd.node) {
				continue
			}
			out = append(out, cd.node)
		}
		for _, c := range d.node.Children {
			if !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
		d.node.Children = out
	}
	for inst, qid := range b.nested {
		q, ok := b.lib.Prefab(qid)
		if !ok {
			continue
		}
		o, err := author.Normalize(b.lib, q, inst.Instance.Overrides)
		if err != nil {
			return err
		}
		inst.Instance.Overrides = o
	}
	return nil
}

func addedIn(inst *prefab.Node, rel prefab.Key) (*prefab.Node, *prefab.Node, bool) {
	s := string(rel)
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	local, err := ids.ParseNodeID(s)
	if err != nil {
		return nil, nil, false
	}
	for _, a := range inst.Instance.Overrides.AddNodes {
		var found, parent *prefab.Node
		a.Node.Walk(func(n, p *prefab.Node) {
			if found == nil && n.Local == local {
				found, parent = n, p
			}
		})
		if found != nil {
			return found, parent, true
		}
	}
	return nil, nil, false
}

func (b *baker) Nearest(id ids.AssetID, key prefab.Key) (ids.AssetID, bool) {
	d, err := b.route(id, key, true, false)
	if err != nil || d.inst != nil {
		return ids.AssetID{}, false
	}
	return d.owner, true
}
