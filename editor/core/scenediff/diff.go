package scenediff

import (
	"sort"
	"strings"

	"aego/format/aetext"
)

type Kind uint8

const (
	NodeAdded Kind = iota
	NodeRemoved
	NodeMoved
	NodeReordered
	NodeRenamed
	NodeAttr
	CompAdded
	CompRemoved
	FieldAdded
	FieldRemoved
	FieldChanged
)

var kindNames = [...]string{"node added", "node removed", "moved", "reordered", "renamed", "node attribute", "component added", "component removed", "field added", "field removed", "changed"}

func (k Kind) String() string {
	return kindNames[k]
}

type Change struct {
	Kind   Kind
	Node   string
	Path   string
	Owner  string
	Field  string
	Before string
	After  string
}

type Result struct {
	Changes []Change
	Scene   bool
}

func (r *Result) Empty() bool {
	return len(r.Changes) == 0
}

type node struct {
	id     string
	name   string
	parent string
	index  int
	attrs  map[string]string
	blocks []*aetext.Node
	comps  map[string]*aetext.Node
	order  []string
}

type tree struct {
	nodes map[string]*node
	order []string
}

func ValueText(v aetext.Value) string {
	out := string(aetext.Write(&aetext.Node{Head: "x", Fields: []aetext.Field{{Name: "v", Value: v}}}))
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(t, "v"); ok && (strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "=")) {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "="))
		}
	}
	return strings.TrimSpace(out)
}

func parse(src []byte, name string) (*aetext.Node, error) {
	if len(src) == 0 {
		return &aetext.Node{}, nil
	}
	return aetext.Parse(src, name)
}

func isScene(doc *aetext.Node) bool {
	return doc.Head == "scene" || doc.Head == "prefab"
}

func build(doc *aetext.Node) *tree {
	t := &tree{nodes: map[string]*node{}}
	var walk func(n *aetext.Node, parent string)
	walk = func(n *aetext.Node, parent string) {
		idx := 0
		for _, b := range n.Blocks {
			if b.Head != "node" {
				continue
			}
			nd := &node{id: b.Label, parent: parent, index: idx, attrs: map[string]string{}, comps: map[string]*aetext.Node{}}
			idx++
			for _, a := range b.Attrs {
				if a.Name == "name" {
					nd.name = a.Value.Str
					continue
				}
				nd.attrs[a.Name] = ValueText(a.Value)
			}
			for _, c := range b.Blocks {
				switch c.Head {
				case "node":
				case "component":
					nd.comps[c.Label] = c
					nd.order = append(nd.order, c.Label)
				default:
					nd.blocks = append(nd.blocks, c)
				}
			}
			t.nodes[nd.id] = nd
			t.order = append(t.order, nd.id)
			walk(b, nd.id)
		}
	}
	walk(doc, "")
	return t
}

func (t *tree) path(id string) string {
	var parts []string
	for n := t.nodes[id]; n != nil; n = t.nodes[n.parent] {
		name := n.name
		if name == "" {
			name = n.id
		}
		parts = append(parts, name)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

func Compute(before, after []byte, name string) (Result, error) {
	a, err := parse(before, name)
	if err != nil {
		return Result{}, err
	}
	b, err := parse(after, name)
	if err != nil {
		return Result{}, err
	}
	if isScene(a) || isScene(b) {
		return scenes(a, b), nil
	}
	var r Result
	flat(&r, "", "", a, b)
	return r, nil
}

func scenes(a, b *aetext.Node) Result {
	r := Result{Scene: true}
	ta, tb := build(a), build(b)
	for _, id := range tb.order {
		nb := tb.nodes[id]
		na, ok := ta.nodes[id]
		path := tb.path(id)
		if !ok {
			r.Changes = append(r.Changes, Change{Kind: NodeAdded, Node: id, Path: path})
			continue
		}
		if na.parent != nb.parent {
			r.Changes = append(r.Changes, Change{Kind: NodeMoved, Node: id, Path: path, Before: ta.path(na.parent), After: tb.path(nb.parent)})
		} else if na.index != nb.index {
			r.Changes = append(r.Changes, Change{Kind: NodeReordered, Node: id, Path: path, Before: itoa(na.index), After: itoa(nb.index)})
		}
		if na.name != nb.name {
			r.Changes = append(r.Changes, Change{Kind: NodeRenamed, Node: id, Path: path, Before: na.name, After: nb.name})
		}
		diffAttrs(&r, id, path, na.attrs, nb.attrs)
		for _, owner := range nb.order {
			cb := nb.comps[owner]
			ca, ok := na.comps[owner]
			if !ok {
				r.Changes = append(r.Changes, Change{Kind: CompAdded, Node: id, Path: path, Owner: owner})
				continue
			}
			flat(&r, id+"\x00"+path, owner, ca, cb)
		}
		for _, owner := range na.order {
			if _, ok := nb.comps[owner]; !ok {
				r.Changes = append(r.Changes, Change{Kind: CompRemoved, Node: id, Path: path, Owner: owner})
			}
		}
		flatBlocks(&r, id+"\x00"+path, "", na.blocks, nb.blocks)
	}
	for _, id := range ta.order {
		if _, ok := tb.nodes[id]; !ok {
			r.Changes = append(r.Changes, Change{Kind: NodeRemoved, Node: id, Path: ta.path(id)})
		}
	}
	for i := range r.Changes {
		c := &r.Changes[i]
		if node, path, ok := strings.Cut(c.Node, "\x00"); ok {
			c.Node, c.Path = node, path
		}
	}
	return r
}

func itoa(i int) string {
	var b [20]byte
	n := len(b)
	if i == 0 {
		return "0"
	}
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}

func diffAttrs(r *Result, id, path string, a, b map[string]string) {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	list := make([]string, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	for _, k := range list {
		if a[k] != b[k] {
			r.Changes = append(r.Changes, Change{Kind: NodeAttr, Node: id, Path: path, Field: k, Before: a[k], After: b[k]})
		}
	}
}

func fieldMap(n *aetext.Node) (map[string]string, []string) {
	m := map[string]string{}
	var order []string
	add := func(prefix string, fs []aetext.Field) {
		for _, f := range fs {
			k := prefix + f.Name
			if _, dup := m[k]; !dup {
				order = append(order, k)
			}
			m[k] = ValueText(f.Value)
		}
	}
	if n != nil {
		add("@", n.Attrs)
		add("", n.Fields)
	}
	return m, order
}

func flat(r *Result, node, owner string, a, b *aetext.Node) {
	fa, oa := fieldMap(a)
	fb, ob := fieldMap(b)
	for _, k := range ob {
		va, had := fa[k]
		switch {
		case !had:
			r.Changes = append(r.Changes, Change{Kind: FieldAdded, Node: node, Owner: owner, Field: k, After: fb[k]})
		case va != fb[k]:
			r.Changes = append(r.Changes, Change{Kind: FieldChanged, Node: node, Owner: owner, Field: k, Before: va, After: fb[k]})
		}
	}
	for _, k := range oa {
		if _, ok := fb[k]; !ok {
			r.Changes = append(r.Changes, Change{Kind: FieldRemoved, Node: node, Owner: owner, Field: k, Before: fa[k]})
		}
	}
	var ba, bb []*aetext.Node
	if a != nil {
		ba = a.Blocks
	}
	if b != nil {
		bb = b.Blocks
	}
	flatBlocks(r, node, owner, ba, bb)
}

func blockKey(n *aetext.Node, seen map[string]int) string {
	k := n.Head
	if n.Label != "" {
		k += " " + n.Label
	}
	seen[k]++
	if c := seen[k]; c > 1 {
		k += "#" + itoa(c)
	}
	return k
}

func flatBlocks(r *Result, node, owner string, a, b []*aetext.Node) {
	ma := map[string]*aetext.Node{}
	sa := map[string]int{}
	var oa []string
	for _, n := range a {
		if n.Head == "node" {
			continue
		}
		k := blockKey(n, sa)
		ma[k] = n
		oa = append(oa, k)
	}
	mb := map[string]*aetext.Node{}
	sb := map[string]int{}
	for _, n := range b {
		if n.Head == "node" {
			continue
		}
		k := blockKey(n, sb)
		mb[k] = n
		sub := join(owner, k)
		if prev, ok := ma[k]; ok {
			flat(r, node, sub, prev, n)
		} else {
			flat(r, node, sub, nil, n)
		}
	}
	for _, k := range oa {
		if _, ok := mb[k]; !ok {
			flat(r, node, join(owner, k), ma[k], nil)
		}
	}
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + " › " + b
}
