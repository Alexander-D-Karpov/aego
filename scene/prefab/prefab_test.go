package prefab_test

import (
	"testing"

	"aego/core/errs"
	"aego/core/ids"
	"aego/scene/prefab"
	"aego/schema"
)

type library map[prefab.PrefabID]*prefab.Node

func (l library) Prefab(id prefab.PrefabID) (*prefab.Node, bool) {
	n, ok := l[id]
	return n, ok
}

func nid(n uint64) ids.NodeID {
	return ids.NodeID{Hi: 0xa11, Lo: n}
}

func instanceOf(local ids.NodeID, p prefab.PrefabID) *prefab.Node {
	return &prefab.Node{Local: local, Instance: &prefab.Instance{Prefab: p}}
}

func TestKeyID(t *testing.T) {
	k := prefab.LocalKey(nid(7))
	if k.ID() != nid(7) || k.Nested() {
		t.Fatal("a single key must be its own local id")
	}
	n := prefab.Rekey(k, prefab.LocalKey(nid(1)), prefab.LocalKey(nid(2)))
	again := prefab.Rekey(k, prefab.LocalKey(nid(1)), prefab.LocalKey(nid(2)))
	if !n.Nested() || n.ID() == nid(7) || n.ID() != again.ID() || n.Instance() != prefab.LocalKey(nid(2)) {
		t.Fatalf("nested key %s", n)
	}
	if got, ok := prefab.ParseKey(string(n)); !ok || got != n {
		t.Fatal("nested key does not parse back")
	}
	if prefab.Rekey(prefab.LocalKey(nid(1)), prefab.LocalKey(nid(1)), prefab.LocalKey(nid(2))) != prefab.LocalKey(nid(2)) {
		t.Fatal("the nested root takes the key of the instance node")
	}
}

func TestNestedKeysAndRefs(t *testing.T) {
	q := prefab.NewNode(nid(1), "Q", false)
	b := prefab.NewNode(nid(2), "B", false)
	q.Children = []*prefab.Node{b}
	q.Comps = append(q.Comps, prefab.Comp{Owner: "test.A", Kind: prefab.CompComponent, Fields: []prefab.Field{
		{Name: "r", Value: schema.Node(b.Local)},
		{Name: "self", Value: schema.Node(q.Local)},
		{Name: "out", Value: schema.Node(nid(99))},
	}})
	qid := ids.AssetID{Lo: 1}
	p := prefab.NewNode(nid(3), "P", false)
	p.Children = []*prefab.Node{instanceOf(nid(4), qid)}
	tree, orph, err := prefab.Expand(library{qid: q}, p, nil)
	if err != nil || !orph.Empty() {
		t.Fatal(err)
	}
	n := tree.Children[0]
	inner := n.Children[0]
	if n.Key != prefab.LocalKey(nid(4)) || n.Local != nid(4) || n.Origin == nil || n.Name() != "Q" {
		t.Fatalf("instance root = %+v", n)
	}
	if inner.Key != prefab.LocalKey(nid(4))+"/"+prefab.LocalKey(nid(2)) || inner.Local != inner.Key.ID() {
		t.Fatalf("inner key = %s", inner.Key)
	}
	if v, _ := n.Get(prefab.PropKey{Owner: "test.A", Field: "r"}); v.Node != inner.Local {
		t.Fatal("ref inside the nested prefab must follow the key")
	}
	if v, _ := n.Get(prefab.PropKey{Owner: "test.A", Field: "self"}); v.Node != nid(4) {
		t.Fatal("ref to the nested root must point at the instance node")
	}
	if v, _ := n.Get(prefab.PropKey{Owner: "test.A", Field: "out"}); v.Node != nid(99) {
		t.Fatal("ref outside the prefab must stay as is")
	}
	if v, _ := q.Comp("test.A").Get("r"); v.Node != nid(2) {
		t.Fatal("expand changed the library prefab")
	}
}

func TestMissingPrefab(t *testing.T) {
	p := prefab.NewNode(nid(1), "P", false)
	qid := ids.AssetID{Lo: 5}
	p.Children = []*prefab.Node{instanceOf(nid(2), qid)}
	tree, _, err := prefab.Expand(library{}, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := tree.Children[0]
	if !m.Missing || m.Origin == nil || m.Origin.Prefab != qid || m.Key != prefab.LocalKey(nid(2)) {
		t.Fatalf("missing node = %+v", m)
	}
}

func TestCycleAndDepth(t *testing.T) {
	a, b := ids.AssetID{Lo: 1}, ids.AssetID{Lo: 2}
	ra := prefab.NewNode(nid(1), "A", false)
	ra.Children = []*prefab.Node{instanceOf(nid(2), b)}
	rb := prefab.NewNode(nid(3), "B", false)
	rb.Children = []*prefab.Node{instanceOf(nid(4), a)}
	if _, _, err := prefab.Expand(library{a: ra, b: rb}, ra, nil); !errs.Has(err, errs.PrefabCycle) {
		t.Fatalf("cycle: %v", err)
	}
	lib := library{}
	var chain []*prefab.Node
	for i := 0; i <= prefab.MaxDepth+1; i++ {
		n := prefab.NewNode(nid(uint64(100+2*i)), "L", false)
		if i > 0 {
			n.Children = []*prefab.Node{instanceOf(nid(uint64(101+2*i)), ids.AssetID{Lo: uint64(i - 1)})}
		}
		lib[ids.AssetID{Lo: uint64(i)}] = n
		chain = append(chain, n)
	}
	if _, _, err := prefab.Expand(lib, chain[prefab.MaxDepth], nil); err != nil {
		t.Fatalf("depth %d must expand: %v", prefab.MaxDepth, err)
	}
	if _, _, err := prefab.Expand(lib, chain[prefab.MaxDepth+1], nil); !errs.Has(err, errs.PrefabDepth) {
		t.Fatalf("depth: %v", err)
	}
}
