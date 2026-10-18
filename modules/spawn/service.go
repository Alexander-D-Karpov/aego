package spawn

import (
	"slices"
	"unsafe"

	"aego/contracts/persistence"
	"aego/contracts/scriptevents"
	"aego/contracts/scriptpool"
	"aego/core/errs"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/save"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/scene/scriptcomp"
	"aego/schema"
)

const poolName = "Pool"

var _ persistence.Spawner = (*Service)(nil)

type compSnap struct {
	td   *schema.TypeDescriptor
	data unsafe.Pointer
}

type nodeSnap struct {
	node    scene.NodeHandle
	enabled bool
	pose    scene.Pose
	comps   []compSnap
}

type member struct {
	prefab ids.AssetID
	nodes  []nodeSnap
	out    bool
}

type Service struct {
	scene   *scene.Scene
	world   *engine.World
	ev      Events
	lib     *sceneio.Library
	tmpl    map[ids.AssetID]*sceneio.Template
	free    map[ids.AssetID][]scene.NodeHandle
	members map[scene.NodeHandle]*member
	parking []scene.NodeHandle
	root    scene.NodeHandle
	unsub   func()
	pids    map[scene.NodeHandle]ids.NodeID
	byPID   map[ids.NodeID]scene.NodeHandle
	order   []scene.NodeHandle
	counter uint64
}

func NewService(s *scene.Scene, w *engine.World, ev Events) *Service {
	svc := &Service{
		scene:   s,
		world:   w,
		ev:      ev,
		lib:     sceneio.NewLibrary(s.Registry(), s.Resolver()),
		tmpl:    map[ids.AssetID]*sceneio.Template{},
		free:    map[ids.AssetID][]scene.NodeHandle{},
		members: map[scene.NodeHandle]*member{},
		pids:    map[scene.NodeHandle]ids.NodeID{},
		byPID:   map[ids.NodeID]scene.NodeHandle{},
	}
	svc.unsub = s.Events().Subscribe(svc.onEvent)
	return svc
}

func (s *Service) Close() {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
}

func (s *Service) onEvent(ev scene.Event) {
	if ev.Kind == scene.EvNodeDestroyed {
		delete(s.members, ev.Node)
		s.unassign(ev.Node)
	}
}

func (s *Service) assign(h scene.NodeHandle, pid ids.NodeID) {
	if old, ok := s.byPID[pid]; ok {
		s.unassign(old)
	}
	s.unassign(h)
	s.pids[h] = pid
	s.byPID[pid] = h
	s.order = append(s.order, h)
	if n := save.Counter(pid); n > s.counter {
		s.counter = n
	}
}

func (s *Service) unassign(h scene.NodeHandle) {
	pid, ok := s.pids[h]
	if !ok {
		return
	}
	delete(s.pids, h)
	delete(s.byPID, pid)
	if i := slices.Index(s.order, h); i >= 0 {
		s.order = slices.Delete(s.order, i, i+1)
	}
}

func (s *Service) PersistentID(h scene.NodeHandle) (ids.NodeID, bool) {
	pid, ok := s.pids[h]
	return pid, ok
}

func (s *Service) ByPersistentID(pid ids.NodeID) (scene.NodeHandle, bool) {
	h, ok := s.byPID[pid]
	return h, ok && s.scene.IsAlive(h)
}

func (s *Service) Prefab(h scene.NodeHandle) (ids.AssetID, bool) {
	m := s.members[h]
	if m == nil {
		return ids.AssetID{}, false
	}
	return m.prefab, true
}

func (s *Service) EachSpawned(fn func(root scene.NodeHandle, prefab ids.AssetID, pid ids.NodeID)) {
	for _, h := range s.order {
		m := s.members[h]
		if m == nil || m.out || !s.scene.IsAlive(h) {
			continue
		}
		fn(h, m.prefab, s.pids[h])
	}
}

func (s *Service) Counter() uint64 {
	return s.counter
}

func (s *Service) SetCounter(n uint64) {
	s.counter = max(s.counter, n)
}

func (s *Service) Pooled(prefab ids.AssetID) int {
	return len(s.free[prefab])
}

func (s *Service) template(id ids.AssetID) (*sceneio.Template, error) {
	if t, ok := s.tmpl[id]; ok {
		return t, nil
	}
	if id.IsZero() {
		return nil, errs.New(errs.AssetNotFound, "spawn: prefab reference is nil")
	}
	t, err := s.lib.Template(id)
	if err != nil {
		return nil, err
	}
	s.tmpl[id] = t
	return t, nil
}

func (s *Service) lifecycle() scriptpool.Lifecycle {
	if s.world == nil {
		return nil
	}
	lc, _ := engine.ServiceOf[scriptpool.Lifecycle](s.world, scriptpool.ServiceID)
	return lc
}

func (s *Service) emit(m *member, ev scriptevents.EventID) {
	if ev == scriptevents.NoEvent || s.world == nil {
		return
	}
	sink, ok := engine.ServiceOf[scriptevents.Sink](s.world, scriptevents.ServiceID)
	if !ok {
		return
	}
	for i := range m.nodes {
		if h := m.nodes[i].node; s.scene.IsAlive(h) {
			sink.Emit(h, ev, nil)
		}
	}
}

func (s *Service) poolRoot() scene.NodeHandle {
	if !s.scene.IsAlive(s.root) {
		s.root = s.scene.Create(s.scene.Root(), poolName)
		s.scene.SetEnabled(s.root, false)
	}
	return s.root
}

func (s *Service) snap(h scene.NodeHandle) nodeSnap {
	sc := s.scene
	ns := nodeSnap{node: h, enabled: sc.Enabled(h), pose: sc.Pose(h)}
	reg := sc.Registry()
	sc.ComponentsOf(h, func(t schema.TypeID) {
		td, ok := reg.ByID(t)
		if !ok || td.Hidden || t == scriptcomp.ScriptTypeID || td.New == nil || td.Copy == nil {
			return
		}
		p, ok := sc.GetByType(h, t)
		if !ok {
			return
		}
		data := td.New()
		td.Copy(data, p)
		ns.comps = append(ns.comps, compSnap{td: td, data: data})
	})
	return ns
}

func (s *Service) capture(h scene.NodeHandle, m *member) {
	m.nodes = append(m.nodes, s.snap(h))
	for c := s.scene.FirstChild(h); !c.IsZero(); c = s.scene.NextSibling(c) {
		s.capture(c, m)
	}
}

func (s *Service) restore(m *member) {
	sc := s.scene
	for i := range m.nodes {
		n := &m.nodes[i]
		if !sc.IsAlive(n.node) {
			continue
		}
		sc.SetEnabled(n.node, i == 0 || n.enabled)
		if i > 0 {
			sc.SetPose(n.node, n.pose)
		}
		for _, c := range n.comps {
			if p, ok := sc.GetByType(n.node, c.td.ID); ok {
				c.td.Copy(p, c.data)
			}
		}
	}
}

func (s *Service) reuse(prefab ids.AssetID) (scene.NodeHandle, *member) {
	list := s.free[prefab]
	for len(list) > 0 {
		h := list[len(list)-1]
		list = list[:len(list)-1]
		if m := s.members[h]; m != nil && s.scene.IsAlive(h) {
			s.free[prefab] = list
			return h, m
		}
	}
	s.free[prefab] = list
	return scene.NodeHandle{}, nil
}

func (s *Service) instantiate(prefab ids.AssetID, parent scene.NodeHandle) (scene.NodeHandle, *member, error) {
	t, err := s.template(prefab)
	if err != nil {
		return scene.NodeHandle{}, nil, err
	}
	h, err := sceneio.Instantiate(s.scene, parent, t)
	if err != nil {
		return scene.NodeHandle{}, nil, err
	}
	m := &member{prefab: prefab}
	s.capture(h, m)
	s.members[h] = m
	return h, m, nil
}

func (s *Service) Spawn(prefab ids.AssetID, parent scene.NodeHandle, pos mathx.Vec2, rot float32) (scene.NodeHandle, error) {
	return s.SpawnAs(prefab, parent, pos, rot, save.PersistentID(s.counter+1))
}

func (s *Service) SpawnAs(prefab ids.AssetID, parent scene.NodeHandle, pos mathx.Vec2, rot float32, pid ids.NodeID) (scene.NodeHandle, error) {
	sc := s.scene
	switch {
	case parent.IsZero():
		parent = sc.Root()
	case !sc.IsAlive(parent):
		return scene.NodeHandle{}, errs.New(errs.SceneDeadNode, "spawn: parent is nil or destroyed")
	}
	h, m := s.reuse(prefab)
	fresh := m == nil
	if fresh {
		var err error
		if h, m, err = s.instantiate(prefab, parent); err != nil {
			return scene.NodeHandle{}, err
		}
	} else {
		if sc.Parent(h) != parent {
			if err := sc.Reparent(h, parent, -1); err != nil {
				return scene.NodeHandle{}, err
			}
		}
		s.restore(m)
	}
	m.out = false
	if !pid.IsZero() {
		s.assign(h, pid)
	}
	if sc.Space(h) == scene.Space2D {
		t := sc.Local(h)
		t.Position, t.Rotation = pos, rot
		sc.SetLocal(h, t)
	}
	if !fresh {
		if lc := s.lifecycle(); lc != nil {
			for i := range m.nodes {
				lc.Revive(m.nodes[i].node)
			}
		}
	}
	s.emit(m, s.ev.Spawned)
	return h, nil
}

func (s *Service) Despawn(h scene.NodeHandle) {
	sc := s.scene
	if !sc.IsAlive(h) {
		return
	}
	m := s.members[h]
	if m == nil {
		sc.Destroy(h)
		return
	}
	if m.out {
		return
	}
	m.out = true
	s.unassign(h)
	s.emit(m, s.ev.Despawned)
	sc.SetEnabled(h, false)
	s.parking = append(s.parking, h)
}

func (s *Service) Pool(prefab ids.AssetID, n int) error {
	root := s.poolRoot()
	for range n {
		h, m, err := s.instantiate(prefab, root)
		if err != nil {
			return err
		}
		m.out = true
		s.scene.SetEnabled(h, false)
		s.parking = append(s.parking, h)
	}
	return nil
}

func (s *Service) parkAll(lc scriptpool.Lifecycle, m *member) bool {
	ok := true
	for i := range m.nodes {
		if h := m.nodes[i].node; s.scene.IsAlive(h) && !lc.Park(h) {
			ok = false
		}
	}
	return ok
}

func (s *Service) Park() {
	if len(s.parking) == 0 {
		return
	}
	lc := s.lifecycle()
	keep := s.parking[:0]
	for _, h := range s.parking {
		m := s.members[h]
		if m == nil || !m.out || !s.scene.IsAlive(h) {
			continue
		}
		if lc != nil && !s.parkAll(lc, m) {
			keep = append(keep, h)
			continue
		}
		if root := s.poolRoot(); s.scene.Parent(h) != root {
			_ = s.scene.Reparent(h, root, -1)
		}
		s.free[m.prefab] = append(s.free[m.prefab], h)
	}
	s.parking = keep
}
