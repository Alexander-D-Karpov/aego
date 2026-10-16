package prefabs

import (
	"aego/core/ids"
	"aego/editor/core/document"
	"aego/scene"
	"aego/scene/prefab"
	"aego/scene/sceneio"
)

type Status uint8

const (
	StatusNone Status = iota
	StatusRoot
	StatusNested
	StatusInside
	StatusAdded
	StatusMissing
)

type Row struct {
	Status  Status
	Orphans bool
}

type Target struct {
	Name    string
	Nearest bool
}

type propRef struct {
	key   prefab.Key
	owner string
	field string
}

type compRef struct {
	key   prefab.Key
	owner string
}

type infoKey struct {
	doc  document.ID
	root ids.NodeID
}

type Info struct {
	Root      ids.NodeID
	Prefab    ids.AssetID
	Path      string
	Missing   bool
	Keys      map[ids.NodeID]prefab.Key
	Overrides prefab.Overrides
	props     map[propRef]bool
	comps     map[compRef]bool
	added     map[ids.NodeID]bool
	addComps  map[compRef]bool
	removed   map[prefab.Key][]string
	touched   map[prefab.Key]bool
	stamp     [3]uint64
}

func (i *Info) KeyOf(node ids.NodeID) prefab.Key {
	if k, ok := i.Keys[node]; ok {
		return k
	}
	return prefab.LocalKey(node)
}

func (i *Info) Overridden(node ids.NodeID, owner, field string) bool {
	return i.props[propRef{key: i.KeyOf(node), owner: owner, field: field}]
}

func (i *Info) Added(node ids.NodeID) bool {
	return i.added[node]
}

func (i *Info) ComponentAdded(node ids.NodeID, owner string) bool {
	return i.addComps[compRef{key: i.KeyOf(node), owner: owner}]
}

func (i *Info) ComponentTouched(node ids.NodeID, owner string) bool {
	return i.comps[compRef{key: i.KeyOf(node), owner: owner}]
}

func (i *Info) Touched(node ids.NodeID) bool {
	return i.added[node] || i.touched[i.KeyOf(node)]
}

func (i *Info) Removed(node ids.NodeID) []string {
	return i.removed[i.KeyOf(node)]
}

func (i *Info) Orphans() prefab.Overrides {
	return i.Overrides.Orphans()
}

func (s *Service) Info(d *document.SceneDocument, node ids.NodeID) (*Info, bool) {
	if d == nil {
		return nil, false
	}
	sc := d.Scene()
	h := sc.Handle(node)
	if h.IsZero() {
		return nil, false
	}
	root, pi, ok := sceneio.OwnerRoot(sc, h)
	if !ok {
		return nil, false
	}
	var gen uint64
	if s.ctx.Loader != nil {
		gen = uint64(s.ctx.Loader.Generation())
	}
	stamp := [3]uint64{d.History.Version(), sc.Revision() ^ sc.StructureRevision()<<32, gen}
	k := infoKey{doc: d.ID, root: sc.ID(root)}
	if inf, ok := s.infos[k]; ok && inf.stamp == stamp {
		return inf, true
	}
	if len(s.infos) > 256 {
		clear(s.infos)
	}
	inf := s.compute(sc, root, pi)
	inf.stamp = stamp
	s.infos[k] = inf
	return inf, true
}

func (s *Service) compute(sc *scene.Scene, root scene.NodeHandle, pi *sceneio.PrefabInstance) *Info {
	inf := &Info{
		Root: sc.ID(root), Prefab: pi.Prefab, Missing: pi.Missing, Keys: pi.Keys,
		props: map[propRef]bool{}, comps: map[compRef]bool{}, added: map[ids.NodeID]bool{},
		addComps: map[compRef]bool{}, removed: map[prefab.Key][]string{}, touched: map[prefab.Key]bool{},
	}
	inf.Path, _ = s.pathOf(pi.Prefab)
	if pi.Missing {
		return inf
	}
	inst, err := sceneio.Overrides(sc, root, s.lib())
	if err != nil {
		return inf
	}
	inf.Overrides = inst.Overrides
	for _, p := range inst.Overrides.Props {
		if p.Orphan {
			continue
		}
		inf.props[propRef{key: p.Node, owner: p.Prop.Owner, field: p.Prop.Field}] = true
		inf.comps[compRef{key: p.Node, owner: p.Prop.Owner}] = true
		inf.touched[p.Node] = true
	}
	for _, a := range inst.Overrides.AddComps {
		if a.Orphan {
			continue
		}
		inf.addComps[compRef{key: a.Node, owner: a.Comp.Owner}] = true
		inf.comps[compRef{key: a.Node, owner: a.Comp.Owner}] = true
		inf.touched[a.Node] = true
	}
	for _, r := range inst.Overrides.RemoveComps {
		inf.removed[r.Node] = append(inf.removed[r.Node], r.Owner)
		inf.touched[r.Node] = true
	}
	for _, o := range inst.Overrides.Order {
		inf.touched[o.Parent] = true
	}
	for _, h := range sc.PreOrder() {
		if h == root || !sc.IsAlive(h) || !under(sc, h, root) {
			continue
		}
		if _, keyed := pi.Keys[sc.ID(h)]; !keyed {
			inf.added[sc.ID(h)] = true
		}
	}
	return inf
}

func under(sc *scene.Scene, h, root scene.NodeHandle) bool {
	for p := sc.Parent(h); !p.IsZero(); p = sc.Parent(p) {
		if p == root {
			return true
		}
	}
	return false
}

func (s *Service) Row(d *document.SceneDocument, node ids.NodeID) Row {
	if d == nil {
		return Row{}
	}
	sc := d.Scene()
	h := sc.Handle(node)
	if h.IsZero() {
		return Row{}
	}
	if pi, ok := sceneio.InstanceOf(sc, h); ok {
		switch {
		case pi.Missing:
			return Row{Status: StatusMissing}
		case pi.Owner:
			return Row{Status: StatusRoot, Orphans: !pi.Orphans.Empty()}
		case pi.Nested:
			return Row{Status: StatusNested}
		}
	}
	_, opi, ok := sceneio.OwnerRoot(sc, h)
	if !ok || !opi.Owner {
		return Row{}
	}
	if _, keyed := opi.Keys[node]; keyed {
		return Row{Status: StatusInside}
	}
	return Row{Status: StatusAdded}
}

func (s *Service) Locked(d *document.SceneDocument, node ids.NodeID) bool {
	if d == nil {
		return false
	}
	sc := d.Scene()
	h := sc.Handle(node)
	root, pi, ok := sceneio.OwnerRoot(sc, h)
	if !ok || !pi.Owner || root == h {
		return false
	}
	_, keyed := pi.Keys[node]
	return keyed
}

func (s *Service) Targets(d *document.SceneDocument, node ids.NodeID) []Target {
	info, ok := s.Info(d, node)
	if !ok || info.Missing {
		return nil
	}
	out := []Target{{Name: s.stem(info.Prefab)}}
	sc := d.Scene()
	root := sc.Handle(info.Root)
	if _, keyed := info.Keys[node]; !keyed {
		return out
	}
	for c := sc.Handle(node); !c.IsZero() && c != root; c = sc.Parent(c) {
		if pi, ok := sceneio.InstanceOf(sc, c); ok && pi.Nested {
			return append([]Target{{Name: s.stem(pi.Prefab), Nearest: true}}, out...)
		}
	}
	return out
}
