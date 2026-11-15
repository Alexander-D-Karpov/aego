package scene

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"aego/core/ids"
	"aego/core/rewind"
)

func TestSnapshotIndependentAcrossStoreGrowthAndReuse(t *testing.T) {
	s := newScene(t)
	hash := func() [32]byte {
		h := rewind.NewHasher()
		s.Hash(h)
		return h.Sum()
	}
	empty := s.Capture(nil)
	emptyHash := hash()
	var nodes []NodeHandle
	for i := range 40 {
		n := s.Create(s.Root(), fmt.Sprintf("one-%d", i))
		nodes = append(nodes, n)
		Add[Health](s, n).Current = float32(i)
		if i%2 == 0 {
			Add[Tag](s, n).Name = fmt.Sprintf("tag-%d", i)
		}
	}
	s.Flush()
	aHash, a := hash(), s.Capture(nil)
	for i, n := range nodes {
		if i%3 == 0 {
			Remove[Health](s, n)
		} else {
			v, _ := Get[Health](s, n)
			v.Current += 100
		}
		if i%4 == 0 {
			Add[Velocity](s, n).Value.X = float32(i)
		}
	}
	s.Flush()
	bHash, b := hash(), s.Capture(nil)
	s.Restore(a)
	if got := hash(); got != aHash {
		t.Fatal("a later independent key changed the earlier snapshot")
	}
	for i := range 100 {
		n := s.Create(s.Root(), fmt.Sprintf("growth-%d", i))
		Add[Velocity](s, n).Value.Y = float32(i)
	}
	s.Flush()
	cHash := hash()
	a = s.Capture(a)
	s.Restore(b)
	if got := hash(); got != bHash {
		t.Fatal("growing or reusing another key changed the second snapshot")
	}
	s.Restore(empty)
	if got := hash(); got != emptyHash {
		t.Fatal("later captures changed an empty snapshot")
	}
	s.Restore(a)
	if got := hash(); got != cHash {
		t.Fatal("reused snapshot did not restore the complete grown scene")
	}
}

func TestSnapshotEmptyStoreAndSparseHistory(t *testing.T) {
	s := newScene(t)
	store := s.Store[Health]()
	empty := s.Capture(nil)
	var nodes []NodeHandle
	for range 12 {
		n := s.Create(s.Root(), "health")
		nodes = append(nodes, n)
		Add[Health](s, n).Current = 7
	}
	s.Flush()
	populated := s.Capture(nil)
	for _, n := range nodes {
		Remove[Health](s, n)
	}
	s.Flush()
	sparse := slices.Clone(store.sparse.dense)
	if len(sparse) == 0 {
		t.Fatal("test requires the removed store to retain a sparse history")
	}
	removed := s.Capture(nil)
	s.Restore(empty)
	if len(store.nodes) != 0 || len(store.data) != 0 || len(store.sparse.dense) != 0 {
		t.Fatal("empty snapshot did not clear a later populated store")
	}
	s.Restore(removed)
	if len(store.nodes) != 0 || len(store.data) != 0 || !slices.Equal(store.sparse.dense, sparse) {
		t.Fatalf("empty store with sparse history was not restored exactly: nodes=%d data=%d sparse=%v, want %v", len(store.nodes), len(store.data), store.sparse.dense, sparse)
	}
	s.Restore(populated)
	for _, n := range nodes {
		h, ok := Get[Health](s, n)
		if !ok || h.Current != 7 {
			t.Fatal("restoring an empty key altered an independent populated key")
		}
	}
}

func TestSnapshotReuseClearsRemovedStores(t *testing.T) {
	first := newScene(t)
	n := first.Create(first.Root(), "first")
	Add[Health](first, n).Current = 1
	Add[Velocity](first, n).Value.X = 2
	Add[Tag](first, n).Name = "three stores"
	first.Flush()
	key := first.Capture(nil).(*sceneKey)
	oldCount := len(key.stores)
	if oldCount != 3 {
		t.Fatalf("expected three stores, got %d", oldCount)
	}
	second := newScene(t)
	n = second.Create(second.Root(), "second")
	Add[Health](second, n).Current = 42
	second.Flush()
	key = second.Capture(key).(*sceneKey)
	for _, stale := range key.stores[len(key.stores):oldCount] {
		if stale != nil {
			t.Fatal("reused snapshot retained a removed store")
		}
	}
	health, _ := Get[Health](second, n)
	health.Current = 0
	second.Restore(key)
	if got, ok := Get[Health](second, n); !ok || got.Current != 42 {
		t.Fatal("cross-scene snapshot reuse lost the current component")
	}
}

func TestSnapshotIndexCacheInvalidatesAndPreservesExactMaps(t *testing.T) {
	s := newScene(t)
	left, right := s.Create(s.Root(), "Left"), s.Create(s.Root(), "Right")
	first, second := s.Create(left, "Shared"), s.Create(right, "Shared")
	s.Flush()
	type recorded struct {
		key   rewind.Key
		ids   map[ids.NodeID]NodeHandle
		names map[nameKey]NodeHandle
	}
	record := func() recorded { return recorded{s.Capture(nil), maps.Clone(s.byID), maps.Clone(s.byName)} }
	initial := record()
	stable := record()
	k1, k2 := initial.key.(*sceneKey), stable.key.(*sceneKey)
	if len(k1.byID) == 0 || len(k1.byName) == 0 || &k1.byID[0] != &k2.byID[0] || &k1.byName[0] != &k2.byName[0] {
		t.Fatal("unchanged indices did not share frozen snapshot data")
	}
	verify := func(r recorded) {
		t.Helper()
		s.Restore(r.key)
		if !maps.Equal(s.byID, r.ids) || !maps.Equal(s.byName, r.names) {
			t.Fatal("restored snapshot indices do not match original maps")
		}
	}
	extra := s.Create(left, "Extra")
	added := record()
	s.SetName(extra, "Renamed")
	renamed := record()
	if err := s.Reparent(first, right, -1); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	collision := record()
	if s.Child(right, "Shared") != first || first == second {
		t.Fatal("collision fixture did not replace the name index")
	}
	s.Destroy(extra)
	pending := record()
	s.Flush()
	removed := record()
	for _, r := range []recorded{initial, added, renamed, collision, pending, removed, stable} {
		verify(r)
	}
	verify(removed)
	s.Capture(nil)
	verify(initial)
	got := record()
	if !maps.Equal(got.ids, initial.ids) || !maps.Equal(got.names, initial.names) {
		t.Fatal("restore reused stale indices")
	}
	old := stable.key
	s.Capture(initial.key)
	verify(recorded{old, stable.ids, stable.names})
}

func TestSnapshotIndexSceneValueCopiesInvalidateSharedMaps(t *testing.T) {
	s := newScene(t)
	child := s.Create(s.Root(), "Before")
	s.Flush()
	alias := *s
	s.Capture(nil)
	alias.SetName(child, "After")
	key := s.Capture(nil)
	s.SetName(child, "Later")
	s.Restore(key)
	if s.Name(child) != "After" || s.Child(s.Root(), "After") != child || !s.Child(s.Root(), "Before").IsZero() {
		t.Fatal("shallow scene copy left stale snapshot indices")
	}
	s.Capture(nil)
	another := *s
	another.SetName(child, "Again")
	key = s.Capture(nil)
	s.SetName(child, "Afterwards")
	s.Restore(key)
	if s.Name(child) != "Again" || s.Child(s.Root(), "Again") != child {
		t.Fatal("post-capture scene copy left stale snapshot indices")
	}
}

func TestSnapshotIndicesStayIndependentAcrossScenes(t *testing.T) {
	first, second := newScene(t), newScene(t)
	first.Create(first.Root(), "First")
	second.Create(second.Root(), "Second")
	first.Flush()
	second.Flush()
	firstIDs, firstNames := maps.Clone(first.byID), maps.Clone(first.byName)
	secondIDs, secondNames := maps.Clone(second.byID), maps.Clone(second.byName)
	reuse, retained := first.Capture(nil), first.Capture(nil)
	reuse = second.Capture(reuse)
	first.Create(first.Root(), "Later")
	second.Create(second.Root(), "Later")
	first.Restore(retained)
	second.Restore(reuse)
	if !maps.Equal(first.byID, firstIDs) || !maps.Equal(first.byName, firstNames) ||
		!maps.Equal(second.byID, secondIDs) || !maps.Equal(second.byName, secondNames) {
		t.Fatal("reusing a snapshot across scenes changed an independent index")
	}
}
