package recorder

import (
	"testing"

	"aego/core/ids"
)

func TestFilterAndRing(t *testing.T) {
	r := New(4)
	a, b := ids.NewNodeID(), ids.NewNodeID()
	r.Stamp(1, 7, 3)
	for range 6 {
		r.Add(Signal, a, ids.NodeID{}, "Died", "")
	}
	got := r.Snapshot(nil)
	if len(got) != 4 || got[0].Frame != 7 || got[0].Tick != 3 || r.Counts()[Signal] != 6 {
		t.Fatalf("ring %v counts %v", len(got), r.Counts())
	}
	r.Clear()
	r.SetFilter(All, []ids.NodeID{b})
	r.Add(Event, a, ids.NodeID{}, "OnCollisionEnter", "")
	r.Add(Event, a, b, "OnCollisionEnter", "")
	if got := r.Snapshot(nil); len(got) != 1 || got[0].Subject != b {
		t.Fatalf("node filter %v", got)
	}
}

func TestCountOnlyIsFree(t *testing.T) {
	r := New(16)
	if n := testing.AllocsPerRun(1000, func() { r.Count(Event) }); n != 0 {
		t.Fatalf("%v allocs", n)
	}
	a := ids.NewNodeID()
	if n := testing.AllocsPerRun(1000, func() { r.Add(Signal, a, ids.NodeID{}, "Died", "") }); n != 0 {
		t.Fatalf("%v allocs", n)
	}
}

func TestBreak(t *testing.T) {
	r := New(16)
	r.BreakOn(Bit(Event), "Trigger")
	r.Add(Event, ids.NodeID{}, ids.NodeID{}, "OnCollisionEnter", "")
	if _, ok := r.Tripped(); ok {
		t.Fatal("tripped on a non-matching name")
	}
	r.Add(Event, ids.NodeID{}, ids.NodeID{}, "OnTriggerEnter", "")
	if rec, ok := r.Tripped(); !ok || rec.Name != "OnTriggerEnter" {
		t.Fatal("break did not trip")
	}
	if _, ok := r.Tripped(); ok {
		t.Fatal("trip was not consumed")
	}
}
