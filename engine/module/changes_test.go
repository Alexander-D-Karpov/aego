package module

import (
	"testing"

	"aego/core/ids"
)

func TestChangeSetDedupes(t *testing.T) {
	var cs ChangeSet
	n := ids.NewNodeID()
	cs.MarkField(n, 1, 10)
	cs.MarkField(n, 1, 10)
	cs.MarkField(n, 1, 11)
	if got := cs.Changes(); len(got) != 1 || len(got[0].Fields) != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestAddThenRemoveCancels(t *testing.T) {
	var cs ChangeSet
	n := ids.NewNodeID()
	cs.MarkAdded(n, 1)
	cs.MarkField(n, 1, 10)
	cs.MarkRemoved(n, 1)
	if !cs.Empty() {
		t.Fatalf("add and remove in one batch should cancel, got %+v", cs.Changes())
	}
}
