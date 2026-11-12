package scene

import (
	"fmt"
	"testing"
)

// A pending removal can be the last dense entry moved over an earlier one.
// It must remain removed, not become visible again during the flush.
func TestBatchRemovalPreservesMovedTombstones(t *testing.T) {
	tests := []struct {
		name    string
		removed []int
		revive  int
	}{
		{name: "last moved first", removed: []int{0, 5}, revive: -1},
		{name: "remove all forward", removed: []int{0, 1, 2, 3, 4, 5}, revive: -1},
		{name: "remove all reverse", removed: []int{5, 4, 3, 2, 1, 0}, revive: -1},
		{name: "multiple moved", removed: []int{1, 4, 5}, revive: -1},
		{name: "revive before flush", removed: []int{0, 3, 5}, revive: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newScene(t)
			var nodes []NodeHandle
			for i := range 6 {
				n := s.Create(s.Root(), fmt.Sprintf("node-%d", i))
				nodes = append(nodes, n)
				Add[Health](s, n).Max = float32(i + 1)
			}
			s.Flush()
			removed := make(map[int]bool)
			for _, i := range tc.removed {
				Remove[Health](s, nodes[i])
				removed[i] = true
			}
			if tc.revive >= 0 {
				if got := Add[Health](s, nodes[tc.revive]); got == nil || got.Max != 0 {
					t.Fatal("revived component is not a new zero value")
				} else {
					got.Max = 99
				}
				delete(removed, tc.revive)
			}
			assertState := func() {
				t.Helper()
				if got, want := Store[Health](s).Len(), len(nodes)-len(removed); got != want {
					t.Errorf("dense length = %d, want %d", got, want)
				}
				visited := 0
				Store[Health](s).EachAll(func(_ NodeHandle, _ *Health) { visited++ })
				if want := len(nodes) - len(removed); visited != want {
					t.Errorf("visited %d components, want %d", visited, want)
				}
				for i, n := range nodes {
					got, ok := Get[Health](s, n)
					if removed[i] {
						if ok || Has[Health](s, n) {
							t.Errorf("removed component %d became visible", i)
						}
						continue
					}
					want := float32(i + 1)
					if i == tc.revive {
						want = 99
					}
					if !ok || got.Max != want {
						t.Errorf("component %d = %v, present %v, want %v", i, got, ok, want)
					}
				}
			}
			s.Flush()
			assertState()
			key := s.Capture(nil)
			for _, n := range nodes {
				Remove[Health](s, n)
			}
			s.Flush()
			s.Restore(key)
			assertState()
		})
	}
}
