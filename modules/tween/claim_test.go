package tween

import (
	"testing"

	"aego/contracts/signals"
	"aego/core/mathx"
	"aego/engine"
	"aego/scene"
	"aego/scene/propclaim"
	"aego/scene/propref"
)

func claimed(t *testing.T, lane int) (*Service, *scene.Scene, scene.NodeHandle, *fakeHub) {
	t.Helper()
	s := newScene(t)
	h := s.Create(s.Root(), "n")
	s.Flush()
	w := engine.NewWorld("t")
	hub := &fakeHub{}
	w.SetService(signals.ServiceID, signals.Hub(hub))
	tb := propclaim.Ensure(w)
	o := tb.Register("test")
	pos := propref.Transform(false, propref.PartPosition).WithLane(lane)
	tb.Begin(o)
	tb.Claim(o, h, &pos)
	tb.End(o)
	return NewService(s, w), s, h, hub
}

func TestClaimedPropertyStopsTween(t *testing.T) {
	svc, s, h, hub := claimed(t, -1)
	tw, err := svc.Position(h, mathx.Vec2{X: 5}, 1, Options{})
	if err != nil {
		t.Fatal(err)
	}
	svc.Step(dt, false)
	if !svc.Done(tw) || s.Local(h).Position != (mathx.Vec2{}) {
		t.Fatalf("done %v, position %v", svc.Done(tw), s.Local(h).Position)
	}
	if len(hub.emits) != 1 || hub.emits[0].sig != sigCompleted || hub.emits[0].arg.Bool() {
		t.Fatalf("emits %+v", hub.emits)
	}
}

func TestOtherLaneIsFree(t *testing.T) {
	svc, s, h, _ := claimed(t, 1)
	tw, err := svc.ToFloat(h, "transform.position.x", 5, 0.25, Options{})
	if err != nil {
		t.Fatal(err)
	}
	steps(svc, 16)
	if !svc.Done(tw) || s.Local(h).Position.X != 5 {
		t.Fatalf("done %v, position %v", svc.Done(tw), s.Local(h).Position)
	}
}
