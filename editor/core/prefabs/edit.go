package prefabs

import (
	"maps"
	"strings"

	"aego/core/errs"
	"aego/core/ids"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/core/history/commands"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

type Level uint8

const (
	LevelProperty Level = iota
	LevelComponent
	LevelNode
	LevelAll
	LevelOrphan
)

type Change struct {
	Node    ids.NodeID
	Key     prefab.Key
	Level   Level
	Prop    prefab.PropKey
	Nearest bool
}

const (
	ownerNode   = "#node"
	ownerRemove = "#remove"
	ownerOrder  = "#order"
)

func (c Change) wants(orphan bool, k prefab.Key, owner, field string, key, rootKey prefab.Key) bool {
	if c.Level == LevelOrphan {
		if !orphan || k != key {
			return false
		}
		return c.Prop.Owner == "" || (owner == c.Prop.Owner && (c.Prop.Field == "" || field == c.Prop.Field))
	}
	if orphan || (k == rootKey && owner == prefab.OwnerTransform) {
		return false
	}
	switch c.Level {
	case LevelProperty:
		return k == key && owner == c.Prop.Owner && field == c.Prop.Field
	case LevelComponent:
		return k == key && owner == c.Prop.Owner
	case LevelNode:
		return k == key
	}
	return true
}

func (c Change) split(o prefab.Overrides, key, rootKey prefab.Key) (sel, rest prefab.Overrides) {
	put := func(take bool, a, b *prefab.Overrides, fn func(*prefab.Overrides)) {
		if take {
			fn(a)
			return
		}
		fn(b)
	}
	for _, p := range o.Props {
		put(c.wants(p.Orphan, p.Node, p.Prop.Owner, p.Prop.Field, key, rootKey), &sel, &rest, func(x *prefab.Overrides) { x.Props = append(x.Props, p) })
	}
	for _, a := range o.AddComps {
		put(c.wants(a.Orphan, a.Node, a.Comp.Owner, "", key, rootKey), &sel, &rest, func(x *prefab.Overrides) { x.AddComps = append(x.AddComps, a) })
	}
	for _, r := range o.RemoveComps {
		put(c.wants(false, r.Node, r.Owner, "", key, rootKey), &sel, &rest, func(x *prefab.Overrides) { x.RemoveComps = append(x.RemoveComps, r) })
	}
	for _, a := range o.AddNodes {
		put(c.wants(a.Orphan, prefab.LocalKey(a.Node.Local), ownerNode, "", key, rootKey), &sel, &rest, func(x *prefab.Overrides) { x.AddNodes = append(x.AddNodes, a) })
	}
	for _, k := range o.Remove {
		put(c.wants(false, k, ownerRemove, "", key, rootKey), &sel, &rest, func(x *prefab.Overrides) { x.Remove = append(x.Remove, k) })
	}
	for _, ord := range o.Order {
		put(c.wants(false, ord.Parent, ownerOrder, "", key, rootKey), &sel, &rest, func(x *prefab.Overrides) { x.Order = append(x.Order, ord) })
	}
	return sel, rest
}

func (s *Service) locate(d *document.SceneDocument, c Change) (scene.NodeHandle, *sceneio.PrefabInstance, prefab.Key, error) {
	if d == nil {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorDocument, "no scene is open")
	}
	sc := d.Scene()
	h := sc.Handle(c.Node)
	if h.IsZero() || !sc.IsAlive(h) {
		return scene.NodeHandle{}, nil, "", errs.New(errs.SceneDeadNode, "node is not in the scene")
	}
	root, pi, ok := sceneio.OwnerRoot(sc, h)
	if !ok {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorCommand, sc.Name(h)+" is not part of a prefab instance")
	}
	if pi.Missing {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorCommand, "the prefab of "+sc.Name(root)+" is missing")
	}
	key := c.Key
	if key == "" {
		if k, ok := pi.Keys[c.Node]; ok {
			key = k
		} else {
			key = prefab.LocalKey(c.Node)
		}
	}
	return root, pi, key, nil
}

func (s *Service) Revert(d *document.SceneDocument, c Change) error {
	if err := s.editable(); err != nil {
		return err
	}
	root, pi, key, err := s.locate(d, c)
	if err != nil {
		return err
	}
	sc := d.Scene()
	lib := s.lib()
	inst, err := sceneio.Overrides(sc, root, lib)
	if err != nil {
		return err
	}
	sel, rest := c.split(inst.Overrides, key, pi.Keys[sc.ID(root)])
	if sel.Empty() {
		return nil
	}
	inst.Overrides = rest
	after := sceneio.EncodeInstance(sc.Registry(), sc.ID(root), inst)
	label := "Revert"
	if c.Level == LevelOrphan {
		label = "Remove Unused Override"
	}
	return d.History.Exec(&commands.Replace{Node: sc.ID(root), Before: sceneio.SaveSubtreeWith(sc, root, lib), After: after, Name: label})
}

func (s *Service) Unpack(d *document.SceneDocument, node ids.NodeID, deep bool) error {
	if err := s.editable(); err != nil {
		return err
	}
	if d == nil {
		return nil
	}
	sc := d.Scene()
	root, pi, ok := sceneio.OwnerRoot(sc, sc.Handle(node))
	if !ok {
		return errs.New(errs.EditorCommand, "the node is not part of a prefab instance")
	}
	cmd := &commands.Replace{
		Node:   sc.ID(root),
		Before: sceneio.SaveSubtree(sc, root, true),
		After:  sceneio.SaveFlat(sc, root, deep),
		Name:   "Unpack Prefab",
	}
	if !deep && !pi.Missing {
		keys := maps.Clone(pi.Keys)
		cmd.Post = func(sc *scene.Scene) { s.promote(sc, keys) }
	}
	if deep {
		cmd.Name = "Unpack Prefab Completely"
	}
	return d.History.Exec(cmd)
}

func (s *Service) promote(sc *scene.Scene, keys map[ids.NodeID]prefab.Key) {
	lib := s.lib()
	for id, k := range keys {
		if k.Nested() {
			continue
		}
		pi, ok := sceneio.InstanceOf(sc, sc.Handle(id))
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

func addPrefix(base *prefab.Node, parent prefab.Key) string {
	if n := base.Find(parent); n != nil && n != base && n.Origin != nil {
		return string(parent)
	}
	return string(parent.Instance())
}

func (s *Service) Apply(d *document.SceneDocument, c Change) error {
	if err := s.editable(); err != nil {
		return err
	}
	if c.Level == LevelOrphan {
		return errs.New(errs.EditorCommand, "unused overrides cannot be applied")
	}
	root, pi, key, err := s.locate(d, c)
	if err != nil {
		return err
	}
	sc := d.Scene()
	lib := s.lib()
	inst, err := sceneio.Overrides(sc, root, lib)
	if err != nil {
		return err
	}
	sel, rest := c.split(inst.Overrides, key, pi.Keys[sc.ID(root)])
	if sel.Empty() {
		s.ctx.Notify(context.LevelInfo, "Nothing to apply")
		return nil
	}
	p, ok := lib.Prefab(pi.Prefab)
	if !ok {
		return errs.New(errs.EditorCommand, "the prefab is not loaded")
	}
	base, _, err := prefab.Expand(lib, p, nil)
	if err != nil {
		return err
	}
	b := newBaker(lib)
	if err := b.bake(pi.Prefab, sel, c.Nearest); err != nil {
		return err
	}
	files, err := s.files(b.edits)
	if err != nil {
		return err
	}
	keep := maps.Clone(inst.IDs)
	for _, a := range sel.AddNodes {
		prefix := addPrefix(base, a.Parent)
		a.Node.Walk(func(n, _ *prefab.Node) {
			if n.Instance != nil {
				return
			}
			k := prefab.LocalKey(n.Local)
			if prefix != "" {
				k = prefab.Key(prefix) + "/" + k
			}
			keep[k] = n.Local
		})
	}
	src := &source{
		doc:    d.ID,
		node:   sc.ID(root),
		before: sceneio.SaveSubtreeWith(sc, root, lib),
		after:  sceneio.EncodeInstance(sc.Registry(), sc.ID(root), &prefab.Instance{Prefab: pi.Prefab, Overrides: rest, IDs: keep}),
	}
	target := s.stem(pi.Prefab)
	if c.Nearest {
		if id, ok := b.Nearest(pi.Prefab, key); ok {
			target = s.stem(id)
		}
	}
	return s.commit("Apply to "+target, files, src)
}
