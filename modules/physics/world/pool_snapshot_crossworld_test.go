package world

import (
	"bytes"
	"testing"
)

func TestCompactSnapshotRestoresIntoSmallerFreshWorld(t *testing.T) {
	w := fallingWorld(t)
	for range 10 {
		w.Step(1.0 / 60)
	}
	key := w.Snapshot()
	config := DefaultConfig()
	config.InitialBodies, config.InitialShapes, config.InitialContacts = 1, 1, 1
	other, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	other.Restore(key)
	if got := other.Snapshot().Bytes(); !bytes.Equal(got, key.Bytes()) {
		t.Fatal("restoring into a smaller world changed the snapshot")
	}
	for range 100 {
		w.Step(1.0 / 60)
		other.Step(1.0 / 60)
	}
	if got, want := other.Snapshot().Bytes(), w.Snapshot().Bytes(); !bytes.Equal(got, want) {
		t.Fatal("cross-world restored simulation diverged")
	}
	d := DefaultBodyDef()
	d.Type = Dynamic
	a, err := w.CreateBody(d)
	if err != nil {
		t.Fatal(err)
	}
	b, err := other.CreateBody(d)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("pool free-list order changed: %v vs %v", a, b)
	}
	shape := ShapeDef{Geometry: Box{Half: Vec2{X: 1, Y: 1}}, Density: 1}
	sa, err := w.AddShape(a, shape)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := other.AddShape(b, shape)
	if err != nil {
		t.Fatal(err)
	}
	if sa != sb {
		t.Fatalf("shape pool free-list order changed: %v vs %v", sa, sb)
	}
	w.Step(1.0 / 60)
	other.Step(1.0 / 60)
	if got, want := other.Snapshot().Bytes(), w.Snapshot().Bytes(); !bytes.Equal(got, want) {
		t.Fatal("adding to restored world after compact capture changed simulation")
	}
}
