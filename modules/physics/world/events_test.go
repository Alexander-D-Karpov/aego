package world

import (
	"slices"
	"testing"
)

func countKind(evs []ContactEvent, k EventKind) int {
	n := 0
	for _, e := range evs {
		if e.Kind == k {
			n++
		}
	}
	return n
}

func dropBox(t *testing.T) (*World, BodyHandle, ShapeHandle, int) {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	b, s := addDynamic(t, w, Vec2{Y: -2}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, nil)
	begins := 0
	for i := 0; i < 120; i++ {
		w.Step(dt)
		evs := w.Events().Contacts
		begins += countKind(evs, BeginContact)
		if countKind(evs, EndContact) > 0 {
			t.Fatalf("step %d: unexpected end", i)
		}
	}
	return w, b, s, begins
}

func TestBeginOncePerPair(t *testing.T) {
	_, _, _, begins := dropBox(t)
	if begins != 1 {
		t.Fatalf("got %d begin events", begins)
	}
}

func TestEndOnSeparation(t *testing.T) {
	w, b, _, _ := dropBox(t)
	noErr(t, w.SetBodyTransform(b, Vec2{Y: -20}, 0))
	w.Step(dt)
	if countKind(w.Events().Contacts, EndContact) != 1 {
		t.Fatalf("events %+v", w.Events().Contacts)
	}
}

func TestEndOnRemoveShapeSameStep(t *testing.T) {
	w, _, s, _ := dropBox(t)
	noErr(t, w.RemoveShape(s))
	if w.Alive(s) {
		t.Fatal("removed shape is alive")
	}
	w.Step(dt)
	evs := w.Events().Contacts
	if countKind(evs, EndContact) != 1 || (evs[0].ShapeA != s && evs[0].ShapeB != s) {
		t.Fatalf("events %+v", evs)
	}
	if w.Alive(s) {
		t.Fatal("removed shape came back")
	}
}

func TestEndOnDestroyBody(t *testing.T) {
	w, b, _, _ := dropBox(t)
	noErr(t, w.DestroyBody(b))
	w.Step(dt)
	if countKind(w.Events().Contacts, EndContact) != 1 {
		t.Fatalf("events %+v", w.Events().Contacts)
	}
}

func TestEndOnTriggerChange(t *testing.T) {
	w, _, s, _ := dropBox(t)
	sd := DefaultShapeDef(Box{Half: Vec2{X: 0.5, Y: 0.5}})
	sd.IsTrigger = true
	noErr(t, w.SetShape(s, sd))
	w.Step(dt)
	evs := w.Events().Contacts
	if countKind(evs, EndContact) != 1 {
		t.Fatalf("events %+v", evs)
	}
}

func TestEndOnFilterChange(t *testing.T) {
	w, b, _, _ := dropBox(t)
	noErr(t, w.SetBodyFilter(b, 0, 0))
	w.Step(dt)
	if countKind(w.Events().Contacts, EndContact) != 1 {
		t.Fatalf("events %+v", w.Events().Contacts)
	}
}

func TestTriggersDoNotCollide(t *testing.T) {
	w := newTestWorld(t, nil)
	zone := addStatic(t, w, Vec2{Y: -5}, 0, Box{Half: Vec2{X: 2, Y: 0.5}}, func(s *ShapeDef) { s.IsTrigger = true })
	ball, _ := addDynamic(t, w, Vec2{Y: -8}, 0, Circle{R: 0.3}, nil)
	var kinds []EventKind
	for i := 0; i < 120; i++ {
		w.Step(dt)
		for _, e := range w.Events().Contacts {
			if e.BodyA == zone || e.BodyB == zone {
				kinds = append(kinds, e.Kind)
			}
		}
	}
	if !slices.Equal(kinds, []EventKind{BeginTrigger, EndTrigger}) {
		t.Fatalf("trigger events %v", kinds)
	}
	if p := posOf(w, ball); p.Y < -4 {
		t.Fatalf("ball stopped at %v", p)
	}
}

func eventLog(t *testing.T) []ContactEvent {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	for i := 0; i < 20; i++ {
		addDynamic(t, w, Vec2{X: float32(i%5) * 0.6, Y: -1 - float32(i)}, float32(i)*0.1, Box{Half: Vec2{X: 0.25, Y: 0.25}}, nil)
	}
	var out []ContactEvent
	for i := 0; i < 240; i++ {
		w.Step(dt)
		out = append(out, w.Events().Contacts...)
	}
	return out
}

func TestEventOrderIsDeterministic(t *testing.T) {
	a, b := eventLog(t), eventLog(t)
	if len(a) == 0 || !slices.Equal(a, b) {
		t.Fatalf("event logs differ: %d vs %d", len(a), len(b))
	}
}
