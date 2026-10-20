package world

import (
	"bytes"
	"testing"
)

func fallingWorld(t *testing.T) *World {
	w, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	g := DefaultBodyDef()
	g.Type = Static
	gh, err := w.CreateBody(g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddShape(gh, ShapeDef{Geometry: Box{Half: Vec2{X: 20, Y: 0.5}}, Friction: 0.6}); err != nil {
		t.Fatal(err)
	}
	box := func(x, y float32) BodyHandle {
		d := DefaultBodyDef()
		d.Type = Dynamic
		d.Position = Vec2{X: x, Y: y}
		h, err := w.CreateBody(d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.AddShape(h, ShapeDef{Geometry: Box{Half: Vec2{X: 0.4, Y: 0.4}}, Density: 1, Friction: 0.5}); err != nil {
			t.Fatal(err)
		}
		return h
	}
	var hs []BodyHandle
	for i := range 6 {
		hs = append(hs, box(float32(i%3)-1, -2-float32(i)))
	}
	if err := w.DestroyBody(hs[1]); err != nil {
		t.Fatal(err)
	}
	w.Step(1.0 / 60)
	box(0.2, -9)
	return w
}

func TestRestoreReplaysBitForBit(t *testing.T) {
	w := fallingWorld(t)
	for range 30 {
		w.Step(1.0 / 60)
	}
	var snap Snapshot
	w.SnapshotInto(&snap)
	for range 120 {
		w.Step(1.0 / 60)
	}
	want := w.Snapshot().Bytes()
	w.Restore(&snap)
	for range 120 {
		w.Step(1.0 / 60)
	}
	if got := w.Snapshot().Bytes(); !bytes.Equal(got, want) {
		t.Fatal("a restored world diverged from the linear run")
	}
}

func TestRestoreKeepsWakeOrder(t *testing.T) {
	w := fallingWorld(t)
	var hs []BodyHandle
	w.ForEachBody(func(h BodyHandle) bool {
		hs = append(hs, h)
		return true
	})
	for range 900 {
		w.Step(1.0 / 60)
	}
	for i := len(hs) - 1; i >= 0; i-- {
		if err := w.WakeUp(hs[i]); err != nil {
			t.Fatal(err)
		}
	}
	var snap Snapshot
	w.SnapshotInto(&snap)
	want := snap.Bytes()
	for range 30 {
		w.Step(1.0 / 60)
	}
	w.Restore(&snap)
	if got := w.Snapshot().Bytes(); !bytes.Equal(got, want) {
		t.Fatal("Restore lost the order in which bodies woke up")
	}
}
