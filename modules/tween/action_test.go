package tween

import (
	"testing"

	"aego/core/mathx"
	"aego/scene"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSequenceRunsInOrder(t *testing.T) {
	svc, s, h := setup(t)
	a := svc.Sequence(scene.NodeHandle{})
	t1, _ := svc.ToFloat(h, "Probe.value", 1, 0.25, Options{})
	t2, _ := svc.ToVec2(h, "Probe.pos", mathx.Vec2{X: 2, Y: 2}, 0.25, Options{})
	must(t, svc.ThenTween(a, t1))
	must(t, svc.Wait(a, 0.25))
	must(t, svc.ThenTween(a, t2))
	steps(svc, 16)
	if p := get(s, h); p.Value != 1 || p.Pos != (mathx.Vec2{}) {
		t.Fatalf("after the first step %+v", p)
	}
	steps(svc, 8)
	if get(s, h).Pos != (mathx.Vec2{}) {
		t.Fatal("the wait was skipped")
	}
	steps(svc, 40)
	if p := get(s, h); p.Pos != (mathx.Vec2{X: 2, Y: 2}) || !svc.ActionDone(a) {
		t.Fatalf("sequence did not finish: %+v", p)
	}
}

func TestParallelEndsAfterLongest(t *testing.T) {
	svc, _, h := setup(t)
	hub := &fakeHub{}
	svc.hub = hub
	a := svc.Parallel(scene.NodeHandle{})
	t1, _ := svc.ToFloat(h, "Probe.value", 1, 0.25, Options{})
	t2, _ := svc.ToVec2(h, "Probe.pos", mathx.Vec2{X: 1}, 0.5, Options{})
	must(t, svc.ThenTween(a, t1))
	must(t, svc.ThenTween(a, t2))
	steps(svc, 20)
	if svc.ActionDone(a) || !svc.Done(t1) {
		t.Fatal("parallel ended too early")
	}
	steps(svc, 12)
	if !svc.ActionDone(a) {
		t.Fatal("parallel did not end after its longest step")
	}
	last := hub.emits[len(hub.emits)-1]
	if last.src != handleSource(typeAction, a.Key()) || !last.arg.Bool() {
		t.Fatalf("Completed %+v", last)
	}
}

func TestThenRules(t *testing.T) {
	svc, _, h := setup(t)
	moved, _ := svc.ToFloat(h, "Probe.value", 1, 1, Options{})
	steps(svc, 1)
	a := svc.Sequence(scene.NodeHandle{})
	if svc.ThenTween(a, moved) == nil {
		t.Fatal("a tween that already moved must be rejected")
	}
	cd, _ := svc.Cooldown(1, TimerOptions{})
	if svc.ThenTimer(a, cd) == nil {
		t.Fatal("a ready cooldown is not a step")
	}
	steps(svc, 1)
	if !svc.ActionDone(a) {
		t.Fatal("an empty action ends when it starts")
	}
	b := svc.Sequence(scene.NodeHandle{})
	steps(svc, 1)
	fresh, _ := svc.ToFloat(h, "Probe.count", 1, 1, Options{})
	if svc.ThenTween(b, fresh) == nil {
		t.Fatal("steps cannot be added to a started action")
	}
}

func TestStopActionStopsHeldSteps(t *testing.T) {
	svc, s, h := setup(t)
	a := svc.Sequence(scene.NodeHandle{})
	t1, _ := svc.ToFloat(h, "Probe.value", 1, 0.5, Options{})
	t2, _ := svc.ToVec2(h, "Probe.pos", mathx.Vec2{X: 1}, 0.5, Options{})
	must(t, svc.ThenTween(a, t1))
	must(t, svc.ThenTween(a, t2))
	steps(svc, 8)
	svc.StopAction(a)
	steps(svc, 40)
	if !svc.ActionDone(a) || !svc.Done(t1) || !svc.Done(t2) || get(s, h).Pos != (mathx.Vec2{}) {
		t.Fatal("Stop must stop the running and the held steps")
	}
}

func TestOwnerDeathStopsAction(t *testing.T) {
	svc, s, h := setup(t)
	owner := s.Create(s.Root(), "owner")
	s.Flush()
	a := svc.Sequence(owner)
	t1, _ := svc.ToFloat(h, "Probe.value", 1, 1, Options{})
	must(t, svc.ThenTween(a, t1))
	steps(svc, 2)
	s.Destroy(owner)
	steps(svc, 1)
	if !svc.ActionDone(a) || !svc.Done(t1) {
		t.Fatal("the action of a dead owner must stop")
	}
}
