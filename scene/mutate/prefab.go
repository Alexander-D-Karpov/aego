package mutate

import (
	"maps"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

func (d *Document) instantiate(op Op, res *Result) (step, error) {
	s := d.Scene
	p, err := d.node(op.Parent)
	if err != nil {
		return step{}, err
	}
	id, err := d.assetID(op.Prefab)
	if err != nil {
		return step{}, err
	}
	lib := d.library()
	t, err := lib.Template(id)
	if err != nil {
		return step{}, err
	}
	h, err := sceneio.Instantiate(s, p, t)
	if err != nil {
		return step{}, err
	}
	s.Flush()
	if op.Name != "" {
		s.SetName(h, op.Name)
	}
	if err := d.place(h, p, op); err != nil {
		s.Destroy(h)
		s.Flush()
		return step{}, err
	}
	res.Created = append(res.Created, idRef(s, h))
	redo := Op{Kind: KindInsert, Parent: idRef(s, p), Fragment: string(d.subtree(h)), Index: childIndex(s, h), HasIndex: true}
	ch := d.change(op.Kind, h)
	ch.Path = id.String()
	return step{inv: RemoveNode(idRef(s, h)), redo: redo, change: ch}, nil
}

func (d *Document) owner(ref string) (scene.NodeHandle, ids.NodeID, *sceneio.PrefabInstance, error) {
	s := d.Scene
	h, err := d.node(ref)
	if err != nil {
		return h, ids.NodeID{}, nil, err
	}
	root, pi, ok := sceneio.OwnerRoot(s, h)
	if !ok {
		return h, ids.NodeID{}, nil, errs.New(errs.EditorCommand, d.nodeName(h)+" is not part of a prefab instance")
	}
	return root, s.ID(h), pi, nil
}

func (d *Document) revertPrefab(op Op) (step, error) {
	s := d.Scene
	root, node, pi, err := d.owner(op.Node)
	if err != nil {
		return step{}, err
	}
	if pi.Missing {
		return step{}, errs.New(errs.EditorCommand, "the prefab of "+d.nodeName(root)+" is missing")
	}
	level, ok := prefab.ParseLevel(op.Level)
	if !ok {
		return step{}, errs.New(errs.EditorCommand, "level is property, component, node, all or orphan")
	}
	key := prefab.Key(op.Key)
	if key == "" {
		if k, ok := pi.Keys[node]; ok {
			key = k
		} else {
			key = prefab.LocalKey(node)
		}
	}
	lib := d.library()
	inst, err := sceneio.Overrides(s, root, lib)
	if err != nil {
		return step{}, err
	}
	pick := prefab.Pick{Level: level, Key: key, Prop: prefab.PropKey{Owner: op.Owner, Field: op.Field}}
	sel, rest := pick.Split(inst.Overrides, pi.Keys[s.ID(root)])
	if sel.Empty() {
		return step{}, ErrNothing
	}
	inst.Overrides = rest
	st, _, err := d.replaceWith(root, sceneio.EncodeInstance(d.reg, s.ID(root), inst), op.Kind)
	return st, err
}

func (d *Document) unpack(op Op) (step, error) {
	s := d.Scene
	root, _, pi, err := d.owner(op.Node)
	if err != nil {
		return step{}, err
	}
	keys := maps.Clone(pi.Keys)
	missing := pi.Missing
	st, nh, err := d.replaceWith(root, sceneio.SaveFlat(s, root, op.Deep), op.Kind)
	if err != nil {
		return step{}, err
	}
	if !op.Deep && !missing {
		d.promote(keys)
		st.redo.Fragment = string(d.subtree(nh))
	}
	return st, nil
}

func (d *Document) promote(keys map[ids.NodeID]prefab.Key) {
	s := d.Scene
	lib := d.library()
	for id, k := range keys {
		if k.Nested() {
			continue
		}
		pi, ok := sceneio.InstanceOf(s, s.Handle(id))
		if !ok || !pi.Nested {
			continue
		}
		q, ok := lib.Prefab(pi.Prefab)
		if !ok {
			continue
		}
		own := map[ids.NodeID]prefab.Key{id: prefab.LocalKey(q.Local)}
		prefix := string(k) + "/"
		for sid, kk := range keys {
			if rest, ok := strings.CutPrefix(string(kk), prefix); ok {
				own[sid] = prefab.Key(rest)
			}
		}
		pi.Owner, pi.Nested, pi.Keys = true, false, own
	}
}
