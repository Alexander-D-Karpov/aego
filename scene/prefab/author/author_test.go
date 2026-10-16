package author

import (
	"math/rand"
	"slices"
	"strconv"
	"testing"

	"aego/core/ids"
	"aego/core/mathx"
	"aego/scene/prefab"
	"aego/schema"
)

type library map[prefab.PrefabID]*prefab.Node

func (l library) Prefab(id prefab.PrefabID) (*prefab.Node, bool) {
	n, ok := l[id]
	return n, ok
}

type gen struct {
	r *rand.Rand
}

func (g *gen) id() ids.NodeID {
	return ids.NodeID{Hi: g.r.Uint64() | 1, Lo: g.r.Uint64()}
}

func (g *gen) f32() schema.Value {
	return schema.Float32(float32(g.r.Intn(2000)-1000) / 8)
}

func (g *gen) vec() schema.Value {
	return schema.Vec2(mathx.Vec2{X: float32(g.r.Intn(64)) / 4, Y: float32(g.r.Intn(64)) / 4})
}

func (g *gen) plain(name string) *prefab.Node {
	n := prefab.NewNode(g.id(), name, false)
	n.Comp(prefab.OwnerTransform).Set(prefab.FieldPosition, g.vec())
	if g.r.Intn(2) == 0 {
		n.Comps = append(n.Comps, prefab.Comp{Owner: "test.A", Kind: prefab.CompComponent, Fields: []prefab.Field{
			{Name: "x", Value: g.f32()},
			{Name: "r", Value: schema.Node(ids.NodeID{})},
		}})
	}
	if g.r.Intn(3) == 0 {
		n.Comps = append(n.Comps, prefab.Comp{Owner: prefab.ScriptOwner("game.S"), Kind: prefab.CompScript, Fields: []prefab.Field{
			{Name: prefab.FieldActive, Value: schema.Bool(true)},
			{Name: "Speed", Value: g.f32()},
		}})
	}
	return n
}

func (g *gen) prefab(lib library, avail []prefab.PrefabID) *prefab.Node {
	root := g.plain("Root")
	nodes := []*prefab.Node{root}
	for i := 0; i < 1+g.r.Intn(5); i++ {
		parent := nodes[g.r.Intn(len(nodes))]
		if parent.Instance != nil {
			continue
		}
		var c *prefab.Node
		if len(avail) > 0 && g.r.Intn(3) == 0 {
			q := avail[g.r.Intn(len(avail))]
			c = &prefab.Node{Local: g.id(), Instance: &prefab.Instance{Prefab: q}}
			c.Instance.Overrides.Props = []prefab.PropOverride{{
				Node:  prefab.LocalKey(lib[q].Local),
				Prop:  prefab.PropKey{Owner: prefab.OwnerTransform, Field: prefab.FieldPosition},
				Value: g.vec(),
			}}
		} else {
			c = g.plain("N" + strconv.Itoa(i))
		}
		parent.Children = append(parent.Children, c)
		nodes = append(nodes, c)
	}
	var locals []ids.NodeID
	root.Walk(func(n, _ *prefab.Node) { locals = append(locals, n.Local) })
	root.Walk(func(n, _ *prefab.Node) {
		if c := n.Comp("test.A"); c != nil {
			c.Set("r", schema.Node(locals[g.r.Intn(len(locals))]))
		}
	})
	return root
}

func (g *gen) library() (library, *prefab.Node) {
	lib := library{}
	var avail []prefab.PrefabID
	var top *prefab.Node
	for i := 0; i < 3; i++ {
		p := g.prefab(lib, avail)
		id := ids.AssetID{Hi: g.r.Uint64(), Lo: g.r.Uint64()}
		lib[id] = p
		avail = append(avail, id)
		top = p
	}
	return lib, top
}

func (g *gen) like(v schema.Value, all []*prefab.Node) schema.Value {
	switch v.Kind {
	case schema.KindFloat32:
		return g.f32()
	case schema.KindVec2:
		return g.vec()
	case schema.KindString:
		return schema.String("s" + strconv.Itoa(g.r.Intn(9)))
	case schema.KindBool:
		return schema.Bool(v.I == 0)
	case schema.KindInt64:
		return schema.Int64(int64(g.r.Intn(8)))
	case schema.KindNodeRef:
		return schema.Node(all[g.r.Intn(len(all))].Local)
	}
	return v
}

func (g *gen) mutate(t *prefab.Node) {
	for i := 0; i < 1+g.r.Intn(6); i++ {
		var all []*prefab.Node
		parents := map[*prefab.Node]*prefab.Node{}
		t.Walk(func(n, p *prefab.Node) {
			all = append(all, n)
			parents[n] = p
		})
		n := all[g.r.Intn(len(all))]
		switch g.r.Intn(6) {
		case 0:
			c := &n.Comps[g.r.Intn(len(n.Comps))]
			f := &c.Fields[g.r.Intn(len(c.Fields))]
			f.Value = g.like(f.Value, all)
		case 1:
			if n.Comp("test.B") == nil {
				n.Comps = append(n.Comps, prefab.Comp{Owner: "test.B", Kind: prefab.CompComponent, Fields: []prefab.Field{
					{Name: "s", Value: schema.String("b")},
				}})
			}
		case 2:
			for j := range n.Comps {
				if n.Comps[j].Kind.Removable() {
					n.Comps = slices.Delete(n.Comps, j, j+1)
					break
				}
			}
		case 3:
			c := g.plain("Added")
			n.Children = slices.Insert(n.Children, g.r.Intn(len(n.Children)+1), c)
		case 4:
			if p := parents[n]; p != nil {
				p.Children = slices.DeleteFunc(p.Children, func(c *prefab.Node) bool { return c == n })
			}
		case 5:
			g.r.Shuffle(len(n.Children), func(a, b int) { n.Children[a], n.Children[b] = n.Children[b], n.Children[a] })
		}
	}
}

func TestModelProperties(t *testing.T) {
	for seed := int64(1); seed <= 100; seed++ {
		g := &gen{r: rand.New(rand.NewSource(seed))}
		lib, p := g.library()
		base, _, err := prefab.Expand(lib, p, nil)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		mut := base.Clone()
		g.mutate(mut)
		o := prefab.Diff(mut, base)
		got, orph, err := prefab.Expand(lib, p, &o)
		if err != nil || !orph.Empty() {
			t.Fatalf("seed %d: expand %v, orphans:\n%s", seed, err, prefab.FormatOverrides(orph))
		}
		if a, b := prefab.Format(got), prefab.Format(mut); a != b {
			t.Fatalf("seed %d: Expand(Diff) differs from the edited tree\n%s\n---\n%s\noverrides:\n%s", seed, a, b, prefab.FormatOverrides(o))
		}
		n, err := Normalize(lib, p, o)
		if err != nil || !Same(n, o) {
			t.Fatalf("seed %d: Normalize changed a diff\n%s\n---\n%s", seed, prefab.FormatOverrides(n), prefab.FormatOverrides(o))
		}
		back, _, err := prefab.Expand(lib, p, &n)
		if err != nil {
			t.Fatal(err)
		}
		if d := prefab.Diff(back, base); !Same(d, n) {
			t.Fatalf("seed %d: prefab.Diff(Expand(Normalize)) != Normalize", seed)
		}
		r, conf, err := Rebase(lib, n, p, p)
		if err != nil || !Same(r, n) || len(conf) != 0 {
			t.Fatalf("seed %d: rebase onto the same prefab is not the identity", seed)
		}
	}
}

func childPrefab() (*prefab.Node, *prefab.Node) {
	g := &gen{r: rand.New(rand.NewSource(7))}
	root := prefab.NewNode(g.id(), "Root", false)
	child := prefab.NewNode(g.id(), "Child", false)
	child.Comps = append(child.Comps, prefab.Comp{Owner: "test.A", Kind: prefab.CompComponent, Fields: []prefab.Field{
		{Name: "x", Value: schema.Float32(1)},
	}})
	root.Children = []*prefab.Node{child}
	return root, child
}

func TestRemovedTargetBecomesOrphan(t *testing.T) {
	root, child := childPrefab()
	lib := library{}
	o, err := Normalize(lib, root, prefab.Overrides{Props: []prefab.PropOverride{{
		Node: prefab.LocalKey(child.Local), Prop: prefab.PropKey{Owner: "test.A", Field: "x"}, Value: schema.Float32(9),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	next := root.Clone()
	next.Children = nil
	got, _, err := Rebase(lib, o, root, next)
	if err != nil {
		t.Fatal(err)
	}
	orph := got.Orphans()
	if len(orph.Props) != 1 || orph.Props[0].Value.F != 9 {
		t.Fatalf("orphans:\n%s", prefab.FormatOverrides(got))
	}
	again, err := Normalize(lib, next, got)
	if err != nil || !Same(again, got) {
		t.Fatal("normalize must keep orphan overrides")
	}
	tree, _, err := prefab.Expand(lib, next, &got)
	if err != nil || len(tree.Children) != 0 {
		t.Fatal("orphans must not be applied")
	}
	got2, _, err := Rebase(lib, got, next, root)
	if err != nil || len(got2.Orphans().Props) != 1 {
		t.Fatal("an orphan stays an orphan until it is removed")
	}
}

func TestRebaseReportsConflicts(t *testing.T) {
	root, child := childPrefab()
	lib := library{}
	key := prefab.LocalKey(child.Local)
	prop := prefab.PropKey{Owner: "test.A", Field: "x"}
	o, err := Normalize(lib, root, prefab.Overrides{Props: []prefab.PropOverride{{Node: key, Prop: prop, Value: schema.Float32(9)}}})
	if err != nil {
		t.Fatal(err)
	}
	next := root.Clone()
	next.Children[0].Comp("test.A").Set("x", schema.Float32(3))
	got, conf, err := Rebase(lib, o, root, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(conf) != 1 || conf[0].Mine.F != 9 || conf[0].After.F != 3 {
		t.Fatalf("conflicts = %+v", conf)
	}
	tree, _, _ := prefab.Expand(lib, next, &got)
	if v, _ := tree.Find(key).Get(prop); v.F != 9 {
		t.Fatal("the instance value wins over the changed prefab")
	}
	next.Children[0].Comp("test.A").Set("x", schema.Float32(9))
	got, _, _ = Rebase(lib, got, root, next)
	for _, p := range got.Props {
		if p.Prop == prop {
			t.Fatal("an override equal to the new prefab value must go away")
		}
	}
}
