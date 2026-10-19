package selection

import (
	"testing"

	"aego/core/ids"
)

func TestHistoryWalksExplicitSelections(t *testing.T) {
	s := New()
	a, b, c := Item{Doc: 1, Node: ids.NewNodeID()}, Item{Doc: 1, Node: ids.NewNodeID()}, Item{Doc: 2, Node: ids.NewNodeID()}
	s.Set(a)
	s.Set(b)
	s.Clear()
	s.Set(c)
	if !s.Back() {
		t.Fatal("back must step to b")
	}
	if it, _ := s.Active(); it != b {
		t.Fatalf("active = %v, want b", it)
	}
	s.Back()
	if it, _ := s.Active(); it != a || s.CanBack() {
		t.Fatal("back must reach a and stop there; empty selections are not steps")
	}
	s.Forward()
	s.Forward()
	if it, _ := s.Active(); it != c || s.CanForward() {
		t.Fatal("forward must return to c")
	}
	s.Prune(2, func(ids.NodeID) bool { return false })
	s.Back()
	if it, _ := s.Active(); it != b {
		t.Fatal("pruning must not add a step")
	}
	s.Set(a)
	if s.CanForward() {
		t.Fatal("a new selection after going back drops the forward steps")
	}
}

func TestHistoryDepth(t *testing.T) {
	s := New()
	for i := 0; i < trailMax+10; i++ {
		s.Set(Item{Doc: 1, Node: ids.NewNodeID()})
	}
	steps := 0
	for s.Back() {
		steps++
	}
	if steps != trailMax-1 {
		t.Fatalf("history keeps %d steps, want %d", steps+1, trailMax)
	}
}
