package world

import (
	"bytes"
	"testing"
)

func TestFreshSnapshotDoesNotCopyUnusedPoolReserve(t *testing.T) {
	w := fallingWorld(t)
	if cap(w.bodies.items) <= len(w.bodies.items) || cap(w.shapes.items) <= len(w.shapes.items) {
		t.Fatal("fixture must contain unused body and shape reserve")
	}
	oldBodies, oldShapes := cap(w.bodies.items), cap(w.shapes.items)
	s := w.Snapshot()
	if got, want := cap(s.bodies.items), len(w.bodies.items); got != want {
		t.Errorf("fresh body snapshot capacity = %d, want %d populated slots", got, want)
	}
	if got, want := cap(s.shapes.items), len(w.shapes.items); got != want {
		t.Errorf("fresh shape snapshot capacity = %d, want %d populated slots", got, want)
	}
	if cap(w.bodies.items) != oldBodies || cap(w.shapes.items) != oldShapes {
		t.Fatal("snapshot altered the active world's reserve")
	}
	before := bytes.Clone(s.Bytes())
	for range 40 {
		w.Step(1.0 / 60)
	}
	if !bytes.Equal(s.Bytes(), before) {
		t.Fatal("subsequent simulation changed an independent snapshot")
	}
	w.Restore(s)
	if got := w.Snapshot().Bytes(); !bytes.Equal(got, before) {
		t.Fatal("restoring a compact snapshot changed world state")
	}
}

func TestCompactSnapshotReuseAfterGrowth(t *testing.T) {
	w := fallingWorld(t)
	var key Snapshot
	w.SnapshotInto(&key)
	original := bytes.Clone(key.Bytes())
	other := w.Snapshot()
	for range 30 {
		d := DefaultBodyDef()
		d.Type = Dynamic
		h, err := w.CreateBody(d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.AddShape(h, ShapeDef{Geometry: Box{Half: Vec2{X: 1, Y: 1}}, Density: 1}); err != nil {
			t.Fatal(err)
		}
	}
	w.SnapshotInto(&key)
	grown := bytes.Clone(key.Bytes())
	if bytes.Equal(grown, original) {
		t.Fatal("fixture failed to grow")
	}
	w.Restore(other)
	if got := w.Snapshot().Bytes(); !bytes.Equal(got, original) {
		t.Fatal("growing a reused key changed an independent snapshot")
	}
	w.Restore(&key)
	if got := w.Snapshot().Bytes(); !bytes.Equal(got, grown) {
		t.Fatal("grown snapshot did not restore")
	}
	w.Restore(other)
	w.SnapshotInto(&key)
	if got := key.Bytes(); !bytes.Equal(got, original) {
		t.Fatal("reusing a grown snapshot for a smaller world changed state")
	}
}
