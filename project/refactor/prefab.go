package refactor

import (
	"bytes"
	"maps"
	"os"
	"slices"
	"strings"

	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/project"
	"aego/project/refs"
	"aego/scene"
	"aego/scene/mutate"
	"aego/scene/prefab"
	"aego/scene/sceneio"
	"aego/schema"
)

type PrefabChange struct {
	Node    ids.NodeID
	Key     prefab.Key
	Level   prefab.Level
	Prop    prefab.PropKey
	Nearest bool
}

type ApplyPlan struct {
	Root   ids.NodeID
	Prefab ids.AssetID
	Target ids.AssetID
	Models map[ids.AssetID]*prefab.Node
	Before []byte
	After  []byte
}

type PrefabResult struct {
	Edits  []Edit
	Result *mutate.Result
}

func PrefabLibrary(reg *schema.Registry, src refs.Source, res asset.Resolver) (*sceneio.Library, []error) {
	lib := sceneio.NewLibrary(reg, res)
	var bad []error
	for _, a := range src.Assets() {
		if a.Kind != asset.ArtPrefab {
			continue
		}
		data, err := os.ReadFile(src.Abs(a.Path))
		if err != nil {
			bad = append(bad, errs.Wrap(err, errs.VFSIO, "cannot read "+a.Path))
			continue
		}
		n, err := sceneio.ParsePrefab(data, a.Path, reg)
		if err != nil {
			bad = append(bad, err)
			continue
		}
		lib.Set(a.ID, n)
	}
	return lib, bad
}

func Locate(s *scene.Scene, node ids.NodeID, key prefab.Key) (scene.NodeHandle, *sceneio.PrefabInstance, prefab.Key, error) {
	h := s.Handle(node)
	if h.IsZero() || !s.IsAlive(h) {
		return scene.NodeHandle{}, nil, "", errs.New(errs.SceneDeadNode, "node is not in the scene")
	}
	root, pi, ok := sceneio.OwnerRoot(s, h)
	if !ok {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorCommand, s.Name(h)+" is not part of a prefab instance")
	}
	if pi.Missing {
		return scene.NodeHandle{}, nil, "", errs.New(errs.EditorCommand, "the prefab of "+s.Name(root)+" is missing")
	}
	if key == "" {
		if k, ok := pi.Keys[node]; ok {
			key = k
		} else {
			key = prefab.LocalKey(node)
		}
	}
	return root, pi, key, nil
}

func addPrefix(base *prefab.Node, parent prefab.Key) string {
	if n := base.Find(parent); n != nil && n != base && n.Origin != nil {
		return string(parent)
	}
	return string(parent.Instance())
}

func PlanApply(s *scene.Scene, lib *sceneio.Library, c PrefabChange) (*ApplyPlan, error) {
	if c.Level == prefab.LevelOrphan {
		return nil, errs.New(errs.EditorCommand, "unused overrides cannot be applied")
	}
	root, pi, key, err := Locate(s, c.Node, c.Key)
	if err != nil {
		return nil, err
	}
	inst, err := sceneio.Overrides(s, root, lib)
	if err != nil {
		return nil, err
	}
	sel, rest := prefab.Pick{Level: c.Level, Key: key, Prop: c.Prop}.Split(inst.Overrides, pi.Keys[s.ID(root)])
	if sel.Empty() {
		return nil, mutate.ErrNothing
	}
	p, ok := lib.Prefab(pi.Prefab)
	if !ok {
		return nil, errs.New(errs.EditorCommand, "the prefab is not loaded")
	}
	base, _, err := prefab.Expand(lib, p, nil)
	if err != nil {
		return nil, err
	}
	b := newBaker(lib)
	if err := b.bake(pi.Prefab, sel, c.Nearest); err != nil {
		return nil, err
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
	plan := &ApplyPlan{
		Root:   s.ID(root),
		Prefab: pi.Prefab,
		Target: pi.Prefab,
		Models: b.edits,
		Before: sceneio.SaveSubtreeWith(s, root, lib),
		After:  sceneio.EncodeInstance(s.Registry(), s.ID(root), &prefab.Instance{Prefab: pi.Prefab, Overrides: rest, IDs: keep}),
	}
	if c.Nearest {
		if id, ok := b.Nearest(pi.Prefab, key); ok {
			plan.Target = id
		}
	}
	return plan, nil
}

func (p *ApplyPlan) Edits(reg *schema.Registry, src refs.Source) ([]Edit, error) {
	var out []Edit
	for id, m := range p.Models {
		rel, ok := src.PathOf(id)
		if !ok {
			return nil, errs.New(errs.EditorDocument, "prefab "+id.Short()+" is not in the project")
		}
		before, err := os.ReadFile(src.Abs(rel))
		if err != nil {
			return nil, errs.Wrap(err, errs.EditorDocument, "cannot read the prefab")
		}
		after := sceneio.EncodePrefab(reg, m)
		if bytes.Equal(before, after) {
			continue
		}
		out = append(out, Edit{ID: id, Path: rel, Before: before, After: after})
	}
	slices.SortFunc(out, func(a, b Edit) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

func instanceBlocks(s *scene.Scene, lib *sceneio.Library, skip ids.NodeID) (map[ids.NodeID][]byte, []ids.NodeID) {
	blocks := map[ids.NodeID][]byte{}
	var order []ids.NodeID
	under := s.Handle(skip)
	for _, h := range s.PreOrder() {
		if h == s.Root() || !s.IsAlive(h) {
			continue
		}
		inside := false
		for q := h; !q.IsZero(); q = s.Parent(q) {
			if q == under {
				inside = true
				break
			}
		}
		if inside {
			continue
		}
		if pi, ok := sceneio.InstanceOf(s, h); ok && pi.Owner && !pi.Missing {
			id := s.ID(h)
			blocks[id] = sceneio.SaveSubtreeWith(s, h, lib)
			order = append(order, id)
		}
	}
	return blocks, order
}

func ApplyPrefab(doc *mutate.Document, lib *sceneio.Library, src refs.Source, c PrefabChange) (*PrefabResult, error) {
	plan, err := PlanApply(doc.Scene, lib, c)
	if err != nil {
		return nil, err
	}
	edits, err := plan.Edits(doc.Scene.Registry(), src)
	if err != nil {
		return nil, err
	}
	blocks, others := instanceBlocks(doc.Scene, lib, plan.Root)
	prev := map[ids.AssetID]*prefab.Node{}
	for id, m := range plan.Models {
		if old, ok := lib.Prefab(id); ok {
			prev[id] = old
		}
		lib.Set(id, m)
	}
	ops := []mutate.Op{mutate.Replace("#"+plan.Root.String(), string(plan.After))}
	for _, id := range others {
		ops = append(ops, mutate.Replace("#"+id.String(), string(blocks[id])))
	}
	res, err := doc.Apply(mutate.Tx{Label: "Apply Prefab", Ops: ops})
	if err != nil {
		for id := range plan.Models {
			if old, ok := prev[id]; ok {
				lib.Set(id, old)
			} else {
				lib.Unset(id)
			}
		}
		return nil, err
	}
	n := len(res.Inverse.Ops)
	res.Inverse.Ops[n-1].Fragment = string(plan.Before)
	for i, id := range others {
		res.Inverse.Ops[n-2-i].Fragment = string(blocks[id])
	}
	return &PrefabResult{Edits: edits, Result: res}, nil
}

func CheckCreate(s *scene.Scene, h scene.NodeHandle) error {
	if h == s.Root() {
		return errs.New(errs.EditorCommand, "the scene root cannot become a prefab")
	}
	if pi, ok := sceneio.InstanceOf(s, h); ok && pi.Owner {
		return errs.New(errs.EditorCommand, s.Name(h)+" is already a prefab instance, use Create Variant")
	}
	if root, _, ok := sceneio.OwnerRoot(s, h); ok && root != h {
		return errs.New(errs.EditorCommand, s.Name(h)+" belongs to a prefab instance, unpack it first")
	}
	return nil
}

func sceneIDOf(s *scene.Scene, k prefab.Key) (ids.NodeID, bool) {
	head, rest, nested := strings.Cut(string(k), "/")
	first, err := ids.ParseNodeID(head)
	if err != nil {
		return ids.NodeID{}, false
	}
	if !nested {
		return first, !s.Handle(first).IsZero()
	}
	if pi, ok := sceneio.InstanceOf(s, s.Handle(first)); ok {
		for sid, kk := range pi.Keys {
			if string(kk) == rest {
				return sid, true
			}
		}
	}
	if local, ok := k.Local(); ok && !s.Handle(local).IsZero() {
		return local, true
	}
	return ids.NodeID{}, false
}

func LinkBlock(s *scene.Scene, h scene.NodeHandle, lib *sceneio.Library, id ids.AssetID) ([]byte, error) {
	p, ok := lib.Prefab(id)
	if !ok {
		return nil, errs.New(errs.EditorCommand, "the new prefab did not import")
	}
	tree, _, err := prefab.Expand(lib, p, nil)
	if err != nil {
		return nil, err
	}
	inst := &prefab.Instance{Prefab: id, IDs: map[prefab.Key]ids.NodeID{}}
	tree.Walk(func(n, _ *prefab.Node) {
		if sid, ok := sceneIDOf(s, n.Key); ok {
			inst.IDs[n.Key] = sid
		}
	})
	return sceneio.EncodeInstance(s.Registry(), s.ID(h), inst), nil
}

func CreatePrefab(pl *pipeline.Pipeline, doc *mutate.Document, lib *sceneio.Library, nodeRef, rel string) (ids.AssetID, *mutate.Result, error) {
	s := doc.Scene
	h, err := doc.Node(nodeRef)
	if err != nil {
		return ids.AssetID{}, nil, err
	}
	if err := CheckCreate(s, h); err != nil {
		return ids.AssetID{}, nil, err
	}
	abs := pl.Abs(rel)
	if _, err := os.Stat(abs); err == nil {
		return ids.AssetID{}, nil, errs.New(errs.VFSBadPath, rel+" already exists")
	}
	src := sceneio.SaveSubtree(s, h, true)
	if err := project.WriteAtomic(abs, src); err != nil {
		return ids.AssetID{}, nil, errs.Wrap(err, errs.VFSIO, "cannot write "+rel)
	}
	undo := func() {
		os.Remove(abs)
		os.Remove(abs + pipeline.MetaSuffix)
		pl.Scan()
	}
	if _, _, err := pl.Scan(); err != nil {
		undo()
		return ids.AssetID{}, nil, err
	}
	id, ok := pl.Lookup(rel)
	if !ok {
		undo()
		return ids.AssetID{}, nil, errs.New(errs.VFSBadPath, rel+" has no importer or is outside assets, scenes, scripts and prefabs", log.F("path", rel))
	}
	batch, err := pl.Import(id)
	if err == nil && batch != nil {
		if ferr, bad := batch.Failed[id]; bad {
			err = ferr
		}
	}
	if err != nil {
		undo()
		return ids.AssetID{}, nil, err
	}
	n, err := sceneio.ParsePrefab(src, rel, s.Registry())
	if err != nil {
		undo()
		return ids.AssetID{}, nil, err
	}
	lib.Set(id, n)
	after, err := LinkBlock(s, h, lib, id)
	if err == nil {
		var res *mutate.Result
		res, err = doc.Apply(mutate.Tx{Label: "Create Prefab", Ops: []mutate.Op{mutate.Replace("#"+s.ID(h).String(), string(after))}})
		if err == nil {
			return id, res, nil
		}
	}
	lib.Unset(id)
	undo()
	return ids.AssetID{}, nil, err
}
