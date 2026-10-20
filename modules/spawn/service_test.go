package spawn

import (
	"testing"

	"aego/contracts/scriptevents"
	"aego/contracts/scriptpool"
	"aego/core/ids"
	"aego/core/mathx"
	"aego/engine"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
	"aego/script/vm"
)

type probe struct {
	Value float32 `aego:"value"`
}

type fakeLife struct {
	parked, revived []scene.NodeHandle
	refuse          int
}

func (f *fakeLife) Park(h scene.NodeHandle) bool {
	if f.refuse > 0 {
		f.refuse--
		return false
	}
	f.parked = append(f.parked, h)
	return true
}

func (f *fakeLife) Revive(h scene.NodeHandle) { f.revived = append(f.revived, h) }

type fakeSink struct {
	events []scriptevents.EventID
}

func (f *fakeSink) Emit(_ scene.NodeHandle, ev scriptevents.EventID, _ []vm.Value) {
	f.events = append(f.events, ev)
}
func (f *fakeSink) Flush() {}

var prefabID = ids.AssetID{Lo: 7}

func setup(t *testing.T) (*Service, *scene.Scene, *fakeLife, *fakeSink) {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := sceneio.RegisterPrefab(types); err != nil {
		t.Fatal(err)
	}
	if err := scene.RegisterComponent[probe](types, schema.Name("test.Probe")); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	s := scene.New(types, nil, scene.Options{})
	w := engine.NewWorld("t")
	life, sink := &fakeLife{}, &fakeSink{}
	w.SetService(scriptpool.ServiceID, scriptpool.Lifecycle(life))
	w.SetService(scriptevents.ServiceID, scriptevents.Sink(sink))
	svc := NewService(s, w, Events{Spawned: 1, Despawned: 2})
	comp := &aetext.Node{Head: "component", Label: "test.Probe", Fields: []aetext.Field{{Name: "value", Value: aetext.Float32(3)}}}
	child := &aetext.Node{Head: "node", Attrs: []aetext.Field{{Name: "name", Value: aetext.String("Child")}}}
	root := &aetext.Node{Head: "node", Attrs: []aetext.Field{{Name: "name", Value: aetext.String("Bullet")}}, Blocks: []*aetext.Node{comp, child}}
	doc := &aetext.Node{Head: "prefab", Attrs: []aetext.Field{{Name: "version", Value: aetext.Int(1)}}, Blocks: []*aetext.Node{root}}
	svc.tmpl[prefabID] = &sceneio.Template{Prefab: prefabID, Doc: aetext.Write(doc)}
	return svc, s, life, sink
}

func TestReuseRestoresTemplateValues(t *testing.T) {
	svc, s, life, sink := setup(t)
	h, err := svc.Spawn(prefabID, scene.NodeHandle{}, mathx.Vec2{X: 1, Y: 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := scene.Get[probe](s, h)
	p.Value = 9
	child := s.FirstChild(h)
	s.SetEnabled(child, false)
	svc.Despawn(h)
	if s.Enabled(h) {
		t.Fatal("Despawn must disable at once")
	}
	svc.Park()
	s.Flush()
	if svc.Pooled(prefabID) != 1 || s.Name(s.Parent(h)) != poolName || len(life.parked) != 2 {
		t.Fatalf("pooled %d parent %s parked %d", svc.Pooled(prefabID), s.Name(s.Parent(h)), len(life.parked))
	}
	h2, _ := svc.Spawn(prefabID, scene.NodeHandle{}, mathx.Vec2{X: 5, Y: 5}, 0)
	s.Flush()
	p, _ = scene.Get[probe](s, h2)
	switch {
	case h2 != h:
		t.Fatal("Spawn did not reuse the pooled copy")
	case p.Value != 3 || !s.Enabled(h) || !s.Enabled(child):
		t.Fatalf("values not restored: %v enabled %v %v", p.Value, s.Enabled(h), s.Enabled(child))
	case s.Local(h).Position != (mathx.Vec2{X: 5, Y: 5}) || s.Parent(h) != s.Root():
		t.Fatal("pose or parent")
	case len(life.revived) != 2:
		t.Fatalf("revived %d", len(life.revived))
	}
	want := []scriptevents.EventID{1, 1, 2, 2, 1, 1}
	if len(sink.events) != len(want) {
		t.Fatalf("events %v", sink.events)
	}
}

func TestDespawnPlainNodeDestroys(t *testing.T) {
	svc, s, _, _ := setup(t)
	h := s.Create(s.Root(), "n")
	svc.Despawn(h)
	if s.IsAlive(h) {
		t.Fatal("a node without a pool must be destroyed")
	}
}

func TestPoolPrewarmsAndWaitsForScripts(t *testing.T) {
	svc, s, life, _ := setup(t)
	life.refuse = 1
	if err := svc.Pool(prefabID, 3); err != nil {
		t.Fatal(err)
	}
	svc.Park()
	if svc.Pooled(prefabID) != 2 {
		t.Fatalf("a copy whose scripts are not created yet must wait, pooled %d", svc.Pooled(prefabID))
	}
	svc.Park()
	before := s.Stats().Nodes
	svc.Spawn(prefabID, scene.NodeHandle{}, mathx.Vec2{}, 0)
	if svc.Pooled(prefabID) != 2 || s.Stats().Nodes != before {
		t.Fatal("Spawn must take a prewarmed copy")
	}
}

func TestPersistentIDs(t *testing.T) {
	svc, s, _, _ := setup(t)
	a, _ := svc.Spawn(prefabID, scene.NodeHandle{}, mathx.Vec2{}, 0)
	b, _ := svc.Spawn(prefabID, scene.NodeHandle{}, mathx.Vec2{}, 0)
	pa, _ := svc.PersistentID(a)
	pb, _ := svc.PersistentID(b)
	if pa != save.PersistentID(1) || pb != save.PersistentID(2) || svc.Counter() != 2 {
		t.Fatalf("pids %v %v counter %d", pa, pb, svc.Counter())
	}
	if h, ok := svc.ByPersistentID(pb); !ok || h != b {
		t.Fatal("lookup by pid")
	}
	svc.Despawn(a)
	if _, ok := svc.PersistentID(a); ok {
		t.Fatal("despawn keeps the pid")
	}
	var seen []ids.NodeID
	svc.EachSpawned(func(_ scene.NodeHandle, _ ids.AssetID, pid ids.NodeID) { seen = append(seen, pid) })
	if len(seen) != 1 || seen[0] != pb {
		t.Fatalf("spawned %v", seen)
	}
	svc.Park()
	s.Flush()
	c, _ := svc.Spawn(prefabID, scene.NodeHandle{}, mathx.Vec2{}, 0)
	if pc, _ := svc.PersistentID(c); pc != save.PersistentID(3) {
		t.Fatalf("reused root got %v", pc)
	}
	d, _ := svc.SpawnAs(prefabID, scene.NodeHandle{}, mathx.Vec2{}, 0, save.PersistentID(10))
	if pd, _ := svc.PersistentID(d); pd != save.PersistentID(10) || svc.Counter() != 10 {
		t.Fatalf("SpawnAs pid %v counter %d", pd, svc.Counter())
	}
	s.Destroy(d)
	s.Flush()
	if _, ok := svc.ByPersistentID(save.PersistentID(10)); ok {
		t.Fatal("destroyed root keeps its pid")
	}
	if _, err := svc.Spawn(ids.AssetID{}, scene.NodeHandle{}, mathx.Vec2{}, 0); err == nil || svc.Counter() != 10 {
		t.Fatal("failed spawn must not burn a number")
	}
}
