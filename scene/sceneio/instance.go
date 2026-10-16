package sceneio

import (
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/scene"
	"aego/scene/prefab"
)

type MissingPrefab struct {
	Node   ids.NodeID
	Prefab ids.AssetID
	Reason string
}

func keySet(nodes map[prefab.Key]*prefab.Node) map[prefab.Key]bool {
	out := make(map[prefab.Key]bool, len(nodes))
	for k := range nodes {
		out[k] = true
	}
	return out
}

func rootName(inst *prefab.Instance, root ids.NodeID) string {
	for k, id := range inst.IDs {
		if id != root {
			continue
		}
		for _, p := range inst.Overrides.Props {
			if p.Node == k && p.Prop.Owner == prefab.OwnerNode && p.Prop.Field == prefab.FieldName {
				return p.Value.S
			}
		}
	}
	return "Missing prefab"
}

func (l *loader) loadInstance(n *aetext.Node, parent scene.NodeHandle) error {
	inst, err := l.readInstance(n)
	if err != nil {
		if l.opt.Strict {
			return err
		}
		l.report.warn(l.name, n.Pos, err.Error())
		return nil
	}
	rootFile := l.localOf(n)
	p, ok := l.lib.Prefab(inst.Prefab)
	if !ok {
		return l.missingInstance(n, parent, rootFile, inst, "prefab "+inst.Prefab.String()+" is not imported")
	}
	base, _, err := prefab.Expand(l.lib, p, nil)
	if err != nil {
		return l.missingInstance(n, parent, rootFile, inst, err.Error())
	}
	tree, orph, err := prefab.Expand(l.lib, p, &inst.Overrides)
	if err != nil {
		return l.missingInstance(n, parent, rootFile, inst, err.Error())
	}
	bn, _ := prefab.Index(base)
	fileOf := map[prefab.Key]ids.NodeID{}
	m := map[ids.NodeID]ids.NodeID{}
	tree.Walk(func(nd, _ *prefab.Node) {
		k := nd.Key
		id, ok := inst.IDs[k]
		switch {
		case nd == tree:
			id = rootFile
		case ok:
		case bn[k] != nil:
			id = ids.NewNodeID()
		default:
			id = k.ID()
		}
		fileOf[k] = id
		m[k.ID()] = id
	})
	tree.MapRefs(m)
	w := &writer{reg: l.reg, lib: l.lib}
	block := w.modelNode(tree, func(nd *prefab.Node) ids.NodeID { return fileOf[nd.Key] }, keySet(bn))
	before := len(l.created)
	if err := l.loadNode(block, parent); err != nil {
		return err
	}
	if len(l.created) == before {
		return nil
	}
	root := l.created[before]
	pi, ok := addInstance(l.scene, root)
	if !ok {
		return nil
	}
	pi.Prefab, pi.Owner, pi.Nested, pi.Missing = inst.Prefab, true, false, false
	pi.Keys = make(map[ids.NodeID]prefab.Key, len(bn))
	for k := range bn {
		fid, ok := fileOf[k]
		if !ok {
			continue
		}
		if h, ok := l.fileIDs[fid]; ok {
			pi.Keys[l.scene.ID(h)] = k
		}
	}
	pi.Orphans = inst.Overrides.Orphans()
	pi.Orphans.Merge(orph)
	pi.Extra, _ = inst.Raw.([]*aetext.Node)
	pi.Block = n.Clone()
	if !orph.Empty() {
		l.report.warn(l.name, n.Pos, "some overrides of "+l.scene.Name(root)+" lost their target in the prefab and are kept as unused")
	}
	return nil
}

func (l *loader) missingInstance(n *aetext.Node, parent scene.NodeHandle, rootFile ids.NodeID, inst *prefab.Instance, why string) error {
	if l.opt.Strict {
		return errs.New(errs.SceneIOStrict, "prefab instance cannot be loaded", log.F("path", l.name), log.F("reason", why))
	}
	var id ids.NodeID
	if l.opt.KeepIDs {
		id = rootFile
	}
	name := rootName(inst, rootFile)
	h := l.scene.CreateWithID(parent, name, id)
	if h.IsZero() {
		return errs.New(errs.SceneIOParse, "cannot create a node", log.F("path", l.name))
	}
	l.created = append(l.created, h)
	l.fileIDs[rootFile] = h
	if pi, ok := addInstance(l.scene, h); ok {
		pi.Prefab, pi.Missing, pi.Owner, pi.Nested = inst.Prefab, true, false, false
		pi.Block = n.Clone()
	}
	l.report.Missing = append(l.report.Missing, MissingPrefab{Node: l.scene.ID(h), Prefab: inst.Prefab, Reason: why})
	l.report.warn(l.name, n.Pos, "prefab instance "+name+" is kept as is: "+why)
	return nil
}

func (w *writer) instance(s *scene.Scene, h scene.NodeHandle, pi *PrefabInstance) *aetext.Node {
	if !pi.Missing {
		if inst, ok := w.computeInstance(s, h, pi); ok {
			return w.instanceBlock(s.ID(h), inst)
		}
	}
	if pi.Block == nil {
		return nil
	}
	b := pi.Block.Clone()
	b.Label = s.ID(h).String()
	return b
}

func (w *writer) computeInstance(s *scene.Scene, h scene.NodeHandle, pi *PrefabInstance) (*prefab.Instance, bool) {
	if w.lib == nil {
		return nil, false
	}
	p, ok := w.lib.Prefab(pi.Prefab)
	if !ok {
		return nil, false
	}
	base, _, err := prefab.Expand(w.lib, p, nil)
	if err != nil {
		return nil, false
	}
	flat := (&writer{reg: w.reg, lib: w.lib, flatten: h}).node(s, h)
	if flat == nil {
		return nil, false
	}
	conv := &loader{reg: w.reg, report: &Report{}, name: "instance"}
	tree, err := conv.readModel(flat)
	if err != nil {
		return nil, false
	}
	m := make(map[ids.NodeID]ids.NodeID, len(pi.Keys))
	for id, k := range pi.Keys {
		m[id] = k.ID()
	}
	tree.Walk(func(nd, _ *prefab.Node) {
		if k, ok := pi.Keys[nd.Local]; ok {
			nd.Key = k
			return
		}
		nd.Key = prefab.LocalKey(nd.Local)
		if nd.Instance != nil {
			nd.Origin, nd.Instance = nd.Instance, nil
		}
	})
	if tree.Key != base.Key {
		return nil, false
	}
	tree.MapRefs(m)
	o := prefab.Diff(tree, base)
	o.Merge(pi.Orphans)
	o.Sort()
	inst := &prefab.Instance{Prefab: pi.Prefab, Overrides: o, IDs: map[prefab.Key]ids.NodeID{}}
	tree.Walk(func(nd, _ *prefab.Node) {
		if k, ok := pi.Keys[nd.Local]; ok {
			inst.IDs[k] = nd.Local
		}
	})
	if len(pi.Extra) > 0 {
		inst.Raw = pi.Extra
	}
	return inst, true
}
