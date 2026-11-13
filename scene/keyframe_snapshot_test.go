package scene

import (
	"fmt"
	"slices"
	"testing"

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
