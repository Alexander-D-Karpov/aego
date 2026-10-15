package world

import "testing"

func restingBox(t *testing.T) (*World, BodyHandle) {
	w := newTestWorld(t, nil)
	groundBox(t, w, nil)
	b, _ := addDynamic(t, w, Vec2{Y: -0.5}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, nil)
	run(w, 60)
	if w.BodyAwake(b) {
		t.Fatal("resting box is still awake after a second")
	}
	if s := w.Stats(); s.Sleeping != 1 || s.Active != 0 {
		t.Fatalf("stats %+v", s)
	}
	return w, b
}

func TestWakeByContact(t *testing.T) {
	w, b := restingBox(t)
	top, _ := addDynamic(t, w, Vec2{Y: -4}, 0, Box{Half: Vec2{X: 0.5, Y: 0.5}}, nil)
	for i := 0; i < 120; i++ {
		w.Step(dt)
		for _, ev := range w.Events().Contacts {
			if ev.Kind == BeginContact && (ev.BodyA == top || ev.BodyB == top) && (ev.BodyA == b || ev.BodyB == b) {
				if !w.BodyAwake(b) {
					t.Fatal("contact did not wake the sleeping box")
				}
				return
			}
		}
	}
	t.Fatal("boxes never touched")
}

func TestWakeByForceTransformJoint(t *testing.T) {
	w, b := restingBox(t)
	noErr(t, w.ApplyForce(b, Vec2{X: 1}))
	if !w.BodyAwake(b) {
		t.Fatal("force did not wake the body")
	}
	w, b = restingBox(t)
	noErr(t, w.SetBodyTransform(b, Vec2{X: 1, Y: -0.5}, 0))
	if !w.BodyAwake(b) {
		t.Fatal("SetBodyTransform did not wake the body")
	}
	w, b = restingBox(t)
	other, _ := addDynamic(t, w, Vec2{X: 3, Y: -3}, 0, Circle{R: 0.2}, nil)
	noErr(t, w.Sleep(other))
	noErr(t, w.Sleep(b))
	must(w.CreateJoint(DistanceJoint{A: b, B: other}))
	if !w.BodyAwake(b) {
		t.Fatal("joint did not wake the body")
	}
}

func TestSleepingBodiesAreSkipped(t *testing.T) {
	w, _ := restingBox(t)
	run(w, 10)
	if s := w.Stats(); s.Active != 0 || s.Islands != 0 {
		t.Fatalf("sleeping world solved islands: %+v", s)
	}
}
