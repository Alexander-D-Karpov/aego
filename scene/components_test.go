package scene

import (
	"testing"

	"aego/core/errs"
	"aego/core/mathx"
	"aego/schema"
)

func TestAddIsVisibleThroughGetImmediately(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")

	h := Add[Health](s, a)
	if h == nil {
		t.Fatal("Add returned nil")
	}
	h.Max = 100
	got, ok := Get[Health](s, a)
	if !ok || got.Max != 100 {
		t.Fatal("Get must see a pending component")
	}
	if !Has[Health](s, a) {
		t.Fatal("Has must see a pending component")
	}
	if Store[Health](s).Len() != 0 {
		t.Fatal("pending component must not be in dense storage yet")
	}
	s.Flush()
	if Store[Health](s).Len() != 1 {
		t.Fatal("Flush must move pending into dense storage")
	}
	got, _ = Get[Health](s, a)
	if got.Max != 100 {
		t.Fatal("data lost during flush")
	}
}

func TestAddIsInvisibleToIterationUntilFlush(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	Add[Health](s, a)
	s.Flush()
	s.UpdateTransforms()

	added := 0
	visited := 0
	Store[Health](s).Each(func(h NodeHandle, _ *Health) {
		visited++
		n := s.Create(s.Root(), "spawned")
		Add[Health](s, n)
		added++
		if visited > 100 {
			t.Fatal("iteration looped over its own additions")
		}
	})
	if visited != 1 || added != 1 {
		t.Fatalf("visited=%d added=%d", visited, added)
	}
	s.Flush()
	s.UpdateTransforms()
	if n := Store[Health](s).Len(); n != 2 {
		t.Fatalf("dense length = %d", n)
	}
}

func TestPendingPointersSurviveMoreAdds(t *testing.T) {
	s := newScene(t)
	first := s.Create(s.Root(), "first")
	p := Add[Health](s, first)
	p.Max = 42
	for i := 0; i < pendingChunk*3; i++ {
		n := s.Create(s.Root(), "n")
		q := Add[Health](s, n)
		q.Max = float32(i)
	}
	if p.Max != 42 {
		t.Fatal("a pending pointer was invalidated by later adds")
	}
	s.Flush()
	got, _ := Get[Health](s, first)
	if got.Max != 42 {
		t.Fatal("pending data lost")
	}
}

func TestRemoveIsImmediateButDeferredInStorage(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(s.Root(), "B")
	Add[Health](s, a)
	Add[Health](s, b)
	s.Flush()
	s.UpdateTransforms()

	Remove[Health](s, a)
	if Has[Health](s, a) {
		t.Fatal("Has must be false right after Remove")
	}
	if _, ok := Get[Health](s, a); ok {
		t.Fatal("Get must be false right after Remove")
	}
	if Store[Health](s).Len() != 2 {
		t.Fatal("storage must not shrink before Flush")
	}
	visited := 0
	Store[Health](s).Each(func(h NodeHandle, _ *Health) {
		if h == a {
			t.Fatal("iteration must skip a removed component")
		}
		visited++
	})
	if visited != 1 {
		t.Fatalf("visited = %d", visited)
	}
	s.Flush()
	if Store[Health](s).Len() != 1 {
		t.Fatalf("dense length after flush = %d", Store[Health](s).Len())
	}
	if !Has[Health](s, b) {
		t.Fatal("swap-remove damaged the surviving component")
	}
}

func TestSwapRemoveKeepsNeighbourData(t *testing.T) {
	s := newScene(t)
	var hs []NodeHandle
	for i := 0; i < 8; i++ {
		n := s.Create(s.Root(), "n")
		c := Add[Health](s, n)
		c.Max = float32(i)
		hs = append(hs, n)
	}
	s.Flush()
	s.UpdateTransforms()

	Remove[Health](s, hs[2])
	Remove[Health](s, hs[5])
	s.Flush()

	for i, h := range hs {
		c, ok := Get[Health](s, h)
		if i == 2 || i == 5 {
			if ok {
				t.Fatalf("component %d survived removal", i)
			}
			continue
		}
		if !ok || c.Max != float32(i) {
			t.Fatalf("component %d damaged: ok=%v value=%v", i, ok, c)
		}
	}
}

func TestAddAfterRemoveInSamePhase(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	c := Add[Health](s, a)
	c.Max = 5
	s.Flush()

	Remove[Health](s, a)
	revived := Add[Health](s, a)
	if revived == nil {
		t.Fatal("Add after Remove returned nil")
	}
	if revived.Max != 0 {
		t.Fatalf("revived component must be zeroed, got %v", revived.Max)
	}
	revived.Max = 9
	s.Flush()
	got, ok := Get[Health](s, a)
	if !ok || got.Max != 9 {
		t.Fatalf("revived component lost: ok=%v value=%v", ok, got)
	}
	if Store[Health](s).Len() != 1 {
		t.Fatalf("dense length = %d", Store[Health](s).Len())
	}
}

func TestRemovePendingComponent(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	Add[Health](s, a)
	Remove[Health](s, a)
	if Has[Health](s, a) {
		t.Fatal("removing a pending component must hide it")
	}
	s.Flush()
	if Store[Health](s).Len() != 0 {
		t.Fatal("a removed pending component must not reach storage")
	}
	Add[Health](s, a)
	s.Flush()
	if Store[Health](s).Len() != 1 {
		t.Fatal("re-adding after removing a pending component failed")
	}
}

func TestDoubleAddReturnsExisting(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	first := Add[Health](s, a)
	first.Max = 7
	second := Add[Health](s, a)
	if second.Max != 7 {
		t.Fatal("second Add must return the pending component")
	}
	s.Flush()
	third := Add[Health](s, a)
	third.Current = 3
	if Store[Health](s).Len() != 1 {
		t.Fatal("Add must not duplicate a dense component")
	}
	got, _ := Get[Health](s, a)
	if got.Max != 7 || got.Current != 3 {
		t.Fatalf("component = %+v", got)
	}
}

func TestAddToDestroyedNodeReturnsNil(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()
	s.Destroy(a)
	if p := Add[Health](s, a); p != nil {
		t.Fatal("Add on a pending-destroy node must return nil")
	}
	s.Flush()
	if p := Add[Health](s, a); p != nil {
		t.Fatal("Add on a dead node must return nil")
	}
}

func TestDestroyDropsComponents(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	child := s.Create(a, "Child")
	Add[Health](s, a)
	Add[Velocity](s, child)
	s.Flush()

	s.Destroy(a)
	s.Flush()
	if Store[Health](s).Len() != 0 || Store[Velocity](s).Len() != 0 {
		t.Fatal("destroying a subtree must drop its components")
	}
}

func TestComponentsOf(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	Add[Health](s, a)
	Add[Tag](s, a)
	s.Flush()

	seen := map[schema.TypeID]bool{}
	s.ComponentsOf(a, func(id schema.TypeID) { seen[id] = true })
	if len(seen) != 2 {
		t.Fatalf("components = %d", len(seen))
	}
	if !seen[Store[Health](s).Type()] || !seen[Store[Tag](s).Type()] {
		t.Fatal("wrong component types reported")
	}
}

func TestPropertyAccess(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	Add[Health](s, a)
	s.Flush()

	typeID := Store[Health](s).Type()
	fd, ok := s.Registry().Field(typeID, "max")
	if !ok {
		t.Fatal("field descriptor missing")
	}
	before := s.Revision()
	if err := s.SetProperty(a, typeID, fd.ID, schema.Float32(55)); err != nil {
		t.Fatal(err)
	}
	if s.Revision() == before {
		t.Fatal("SetProperty must bump the revision")
	}
	if !s.Dirty() {
		t.Fatal("scene must be dirty after SetProperty")
	}
	v, err := s.GetProperty(a, typeID, fd.ID)
	if err != nil || v.AsFloat() != 55 {
		t.Fatalf("GetProperty = %+v %v", v, err)
	}
	s.MarkSaved()
	if s.Dirty() {
		t.Fatal("MarkSaved must clear the dirty flag")
	}

	b := s.Create(s.Root(), "B")
	s.Flush()
	if err := s.SetProperty(b, typeID, fd.ID, schema.Float32(1)); !errs.Has(err, errs.SceneNoComponent) {
		t.Fatalf("err = %v", err)
	}
}

func TestByTypeAccess(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	typeID := Store[Health](s).Type()

	p, err := s.AddByType(a, typeID)
	if err != nil || p == nil {
		t.Fatalf("AddByType: %v", err)
	}
	if _, ok := s.GetByType(a, typeID); !ok {
		t.Fatal("GetByType must see the pending component")
	}
	s.Flush()
	s.RemoveByType(a, typeID)
	if _, ok := s.GetByType(a, typeID); ok {
		t.Fatal("RemoveByType had no effect")
	}
	if _, err := s.AddByType(a, schema.TypeID(12345)); !errs.Has(err, errs.SceneUnknownComponent) {
		t.Fatalf("err = %v", err)
	}
}

func TestQueries(t *testing.T) {
	s := newScene(t)
	both := s.Create(s.Root(), "Both")
	onlyHealth := s.Create(s.Root(), "Health")
	onlyVelocity := s.Create(s.Root(), "Velocity")
	Add[Health](s, both)
	Add[Velocity](s, both)
	Add[Health](s, onlyHealth)
	Add[Velocity](s, onlyVelocity)
	s.Flush()
	s.UpdateTransforms()

	if n := Query1[Health](s).Count(); n != 2 {
		t.Fatalf("Query1 count = %d", n)
	}
	seen := 0
	Query2[Health, Velocity](s).Each(func(h NodeHandle, _ *Health, _ *Velocity) {
		if h != both {
			t.Fatalf("unexpected node %q", s.Name(h))
		}
		seen++
	})
	if seen != 1 {
		t.Fatalf("Query2 visited %d nodes", seen)
	}
	Query2[Velocity, Health](s).Each(func(h NodeHandle, _ *Velocity, _ *Health) {
		if h != both {
			t.Fatal("reversed query must give the same result")
		}
	})
}

func TestQuerySkipsDisabledAndPendingDestroy(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(s.Root(), "B")
	c := s.Create(s.Root(), "C")
	for _, h := range []NodeHandle{a, b, c} {
		Add[Health](s, h)
	}
	s.Flush()
	s.UpdateTransforms()

	s.SetEnabled(b, false)
	s.UpdateTransforms()
	s.Destroy(c)

	var visited []NodeHandle
	Query1[Health](s).Each(func(h NodeHandle, _ *Health) {
		visited = append(visited, h)
	})
	if len(visited) != 1 || visited[0] != a {
		t.Fatalf("visited = %v", names(s, visited))
	}
	all := 0
	Store[Health](s).EachAll(func(NodeHandle, *Health) { all++ })
	if all != 3 {
		t.Fatalf("EachAll visited %d", all)
	}
}

func TestEventsFire(t *testing.T) {
	s := newScene(t)
	var kinds []EventKind
	stop := s.Events().Subscribe(func(e Event) {
		kinds = append(kinds, e.Kind)
	})

	a := s.Create(s.Root(), "A")
	Add[Health](s, a)
	s.Flush()
	Remove[Health](s, a)
	s.SetName(a, "B")
	s.Destroy(a)
	s.Flush()
	stop()
	s.Create(s.Root(), "After")

	want := map[EventKind]bool{
		EvNodeCreated:      true,
		EvComponentAdded:   true,
		EvComponentRemoved: true,
		EvNodeRenamed:      true,
		EvNodeDestroyed:    true,
		EvHierarchyChanged: true,
	}
	got := map[EventKind]bool{}
	for _, k := range kinds {
		got[k] = true
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("event %s never fired; got %v", k, kinds)
		}
	}
	n := len(kinds)
	if n == 0 {
		t.Fatal("no events")
	}
	if len(kinds) != n {
		t.Fatal("unsubscribe failed")
	}
}

func TestStatsAndVec(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	v := Add[Velocity](s, a)
	v.Value = mathx.Vec2{X: 1, Y: 2}
	s.Flush()

	st := s.Stats()
	if st.Components != 1 || st.Alive < 2 {
		t.Fatalf("stats = %+v", st)
	}
}
