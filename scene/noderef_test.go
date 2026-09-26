package scene

import (
	"testing"

	"aego/core/ids"
)

func TestNodeRefResolves(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()

	ref := RefTo(s, a)
	if ref.ID != s.ID(a) {
		t.Fatal("ref did not take the node id")
	}
	h, ok := ref.Resolve(s)
	if !ok || h != a {
		t.Fatalf("resolve = %v %v", h, ok)
	}
}

func TestNodeRefSurvivesUnrelatedChanges(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()
	ref := RefTo(s, a)

	for i := 0; i < 5; i++ {
		b := s.Create(s.Root(), "B")
		s.Flush()
		s.Destroy(b)
		s.Flush()
		if h, ok := ref.Resolve(s); !ok || h != a {
			t.Fatalf("iteration %d: resolve = %v %v", i, h, ok)
		}
	}
}

func TestNodeRefBreaksWhenTargetDies(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	s.Flush()
	ref := RefTo(s, a)
	if _, ok := ref.Resolve(s); !ok {
		t.Fatal("precondition failed")
	}

	s.Destroy(a)
	if _, ok := ref.Resolve(s); ok {
		t.Fatal("a pending-destroy target must not resolve")
	}
	s.Flush()
	if _, ok := ref.Resolve(s); ok {
		t.Fatal("a destroyed target must not resolve")
	}
	if ref.ID.IsZero() {
		t.Fatal("the id must survive so it can be saved")
	}
}

func TestNodeRefResolvesLater(t *testing.T) {
	s := newScene(t)
	id := ids.NewNodeID()
	ref := NewNodeRef(id)
	if _, ok := ref.Resolve(s); ok {
		t.Fatal("unknown id must not resolve")
	}
	h := s.CreateWithID(s.Root(), "Later", id)
	s.Flush()
	got, ok := ref.Resolve(s)
	if !ok || got != h {
		t.Fatal("ref must pick up a node created after it")
	}
}

func TestNodeRefSetAndClear(t *testing.T) {
	s := newScene(t)
	a := s.Create(s.Root(), "A")
	b := s.Create(s.Root(), "B")
	s.Flush()

	var ref NodeRef
	ref.Set(s, a)
	if h, _ := ref.Resolve(s); h != a {
		t.Fatal("Set failed")
	}
	ref.Set(s, b)
	if h, _ := ref.Resolve(s); h != b {
		t.Fatal("re-Set failed")
	}
	ref.SetNodeRefID(s.ID(a))
	if h, ok := ref.Resolve(s); !ok || h != a {
		t.Fatal("SetNodeRefID must invalidate the cached handle")
	}
	ref.Clear()
	if !ref.IsZero() {
		t.Fatal("Clear failed")
	}
}

func TestNodeRefImplementsSchemaInterface(t *testing.T) {
	var ref NodeRef
	var iface interface {
		NodeRefID() ids.NodeID
		SetNodeRefID(ids.NodeID)
	} = &ref
	id := ids.NewNodeID()
	iface.SetNodeRefID(id)
	if iface.NodeRefID() != id {
		t.Fatal("interface round trip failed")
	}
}
