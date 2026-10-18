package tween

import (
	"testing"

	"aego/contracts/scriptwait"
	"aego/core/curve"
	"aego/core/mathx"
	"aego/engine"
	"aego/scene"
	"aego/schema"
	"aego/script/abi"
)

const dt = float32(1) / 64

type probe struct {
	Value float32     `aego:"value"`
	Count int32       `aego:"count"`
	Tint  mathx.Color `aego:"tint"`
	Pos   mathx.Vec2  `aego:"pos"`
	Name  string      `aego:"name"`
}

type waker struct {
	calls []uint64
	types []uint64
}

func (w *waker) Complete(typ, id uint64) {
	w.types = append(w.types, typ)
	w.calls = append(w.calls, id)
}

func newScene(t *testing.T) *scene.Scene {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[probe](types, schema.Name("test.Probe")); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return scene.New(types, nil, scene.Options{})
}

func setup(t *testing.T) (*Service, *scene.Scene, scene.NodeHandle) {
	t.Helper()
	s := newScene(t)
	h := s.Create(s.Root(), "n")
	scene.Add[probe](s, h)
	s.Flush()
	return NewService(s, nil), s, h
}

func get(s *scene.Scene, h scene.NodeHandle) *probe {
	p, _ := scene.Get[probe](s, h)
	return p
}

func steps(svc *Service, n int) {
	for range n {
		svc.Step(dt, false)
	}
}

func TestLinearEndsExactlyWithoutOvershoot(t *testing.T) {
	svc, s, h := setup(t)
	tw, err := svc.ToFloat(h, "Probe.value", 10, 1, Options{})
	if err != nil {
		t.Fatal(err)
	}
	prev := float32(0)
	for range 63 {
		svc.Step(dt, false)
		v := get(s, h).Value
		if v < prev || v > 10 {
			t.Fatalf("value %v after %v", v, prev)
		}
		prev = v
	}
	if svc.Done(tw) {
		t.Fatal("finished early")
	}
	svc.Step(dt, false)
	if v := get(s, h).Value; v != 10 || !svc.Done(tw) {
		t.Fatalf("end value %v, done %v", v, svc.Done(tw))
	}
}

func TestDelayReadsStartWhenItEnds(t *testing.T) {
	svc, s, h := setup(t)
	if _, err := svc.ToFloat(h, "Probe.value", 4, 1, Options{Delay: 0.5}); err != nil {
		t.Fatal(err)
	}
	steps(svc, 16)
	get(s, h).Value = 2
	steps(svc, 16)
	if v := get(s, h).Value; v != 2 {
		t.Fatalf("value moved during the delay: %v", v)
	}
	steps(svc, 32)
	if v := get(s, h).Value; v != 3 {
		t.Fatalf("halfway from 2 to 4 is 3, got %v", v)
	}
}

func TestPingPongEndsAtStart(t *testing.T) {
	svc, s, h := setup(t)
	get(s, h).Value = 1
	tw, _ := svc.ToFloat(h, "Probe.value", 5, 0.5, Options{Legs: 2, PingPong: true})
	steps(svc, 32)
	if v := get(s, h).Value; v != 5 {
		t.Fatalf("after the first leg %v", v)
	}
	steps(svc, 32)
	if v := get(s, h).Value; v != 1 || !svc.Done(tw) {
		t.Fatalf("ping-pong must end at the start, got %v", v)
	}
}

func TestStopKeepsCurrentValue(t *testing.T) {
	svc, s, h := setup(t)
	tw, _ := svc.ToFloat(h, "Probe.value", 8, 1, Options{Legs: -1, PingPong: true})
	steps(svc, 96)
	at := get(s, h).Value
	svc.Stop(tw)
	steps(svc, 10)
	if v := get(s, h).Value; v != at || v == 8 || !svc.Done(tw) {
		t.Fatalf("stop jumped: %v → %v", at, v)
	}
}

func TestDestroyedNodeStopsTween(t *testing.T) {
	svc, s, h := setup(t)
	tw, _ := svc.ToFloat(h, "Probe.value", 1, 1, Options{})
	s.Destroy(h)
	svc.Step(dt, false)
	if !svc.Done(tw) || svc.Tweens() != 0 {
		t.Fatal("tween of a destroyed node is still running")
	}
}

func TestUnscaledRunsOnlyInItsDomain(t *testing.T) {
	svc, s, h := setup(t)
	svc.ToFloat(h, "Probe.value", 1, 1, Options{Unscaled: true})
	steps(svc, 16)
	if get(s, h).Value != 0 {
		t.Fatal("game time moved an unscaled tween")
	}
	for range 16 {
		svc.Step(dt, true)
	}
	if v := get(s, h).Value; v != 0.25 {
		t.Fatalf("unscaled value %v", v)
	}
}

func TestLanesNamesAndKinds(t *testing.T) {
	svc, s, h := setup(t)
	get(s, h).Tint = mathx.Color{R: 1, G: 1, B: 1, A: 1}
	if _, err := svc.ToFloat(h, "Probe.Tint.a", 0, 0.25, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToFloat(h, "test.Probe.count", 5, 0.25, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToVec2(h, "Probe.pos", mathx.Vec2{X: 2, Y: -2}, 0.25, Options{}); err != nil {
		t.Fatal(err)
	}
	steps(svc, 16)
	p := get(s, h)
	if p.Tint != (mathx.Color{R: 1, G: 1, B: 1}) || p.Count != 5 || p.Pos != (mathx.Vec2{X: 2, Y: -2}) {
		t.Fatalf("probe %+v", p)
	}
	if _, err := svc.ToVec2(h, "Probe.value", mathx.Vec2{}, 1, Options{}); err == nil {
		t.Fatal("a Vec2 tween on a float field must fail")
	}
	if _, err := svc.ToFloat(h, "Probe.name", 1, 1, Options{}); err == nil {
		t.Fatal("a string field must not tween")
	}
	if _, err := svc.ToFloat(h, "Nope.value", 1, 1, Options{}); err == nil {
		t.Fatal("unknown component accepted")
	}
}

func TestTransformPosition(t *testing.T) {
	svc, s, h := setup(t)
	if _, err := svc.Position(h, mathx.Vec2{X: 4, Y: 8}, 0.5, Options{Ease: curve.InOutSine}); err != nil {
		t.Fatal(err)
	}
	steps(svc, 32)
	if p := s.Local(h).Position; p != (mathx.Vec2{X: 4, Y: 8}) {
		t.Fatalf("position %+v", p)
	}
}

func TestNewTweenReplacesRunningOne(t *testing.T) {
	svc, s, h := setup(t)
	a, _ := svc.ToFloat(h, "Probe.value", 10, 1, Options{})
	steps(svc, 16)
	b, _ := svc.ToFloat(h, "Probe.value", 0, 0.25, Options{})
	if !svc.Done(a) || svc.Done(b) {
		t.Fatal("the second tween must stop the first")
	}
	steps(svc, 16)
	if v := get(s, h).Value; v != 0 {
		t.Fatalf("value %v", v)
	}
}

func TestCurveTween(t *testing.T) {
	svc, s, h := setup(t)
	c := curve.Keyed(curve.Key{T: 0, V: 0, Interp: curve.InterpStep}, curve.Key{T: 1, V: 1})
	svc.ToFloat(h, "Probe.value", 10, 1, Options{Curve: &c})
	steps(svc, 32)
	if v := get(s, h).Value; v != 0 {
		t.Fatalf("a step curve holds the start, got %v", v)
	}
	steps(svc, 32)
	if v := get(s, h).Value; v != 10 {
		t.Fatalf("end %v", v)
	}
}

func TestWakerIsCalled(t *testing.T) {
	s := newScene(t)
	h := s.Create(s.Root(), "n")
	scene.Add[probe](s, h)
	s.Flush()
	w := engine.NewWorld("t")
	wk := &waker{}
	w.SetService(scriptwait.ServiceID, scriptwait.Waker(wk))
	svc := NewService(s, w)
	tw, _ := svc.ToFloat(h, "Probe.value", 1, 0.25, Options{})
	tm, _ := svc.StartTimer(0.25, TimerOptions{Repeat: 2})
	steps(svc, 16)
	if len(wk.calls) != 2 || wk.calls[0] != tw.Key() || wk.types[0] != abi.HandleID(KindTween) ||
		wk.calls[1] != tm.Key() || wk.types[1] != abi.HandleID(KindTimer) {
		t.Fatalf("waker calls %v %v", wk.types, wk.calls)
	}
	steps(svc, 16)
	if len(wk.calls) != 3 || !svc.TimerDone(tm) {
		t.Fatalf("second fire: %v", wk.calls)
	}
}

func TestTimers(t *testing.T) {
	svc, _, _ := setup(t)
	tm, _ := svc.StartTimer(0.5, TimerOptions{Repeat: 2})
	steps(svc, 32)
	if svc.Fired(tm) != 1 || svc.TimerDone(tm) {
		t.Fatalf("fired %d", svc.Fired(tm))
	}
	svc.Pause(tm)
	steps(svc, 64)
	if svc.Fired(tm) != 1 {
		t.Fatal("a paused timer fired")
	}
	svc.Resume(tm)
	steps(svc, 32)
	if !svc.TimerDone(tm) || svc.Timers() != 0 {
		t.Fatal("a finished timer without Keep must free its slot")
	}
	cd, _ := svc.Cooldown(0.25, TimerOptions{})
	if !svc.Ready(cd) || !svc.Use(cd) || svc.Ready(cd) || svc.Use(cd) {
		t.Fatal("cooldown starts ready and is used once")
	}
	steps(svc, 16)
	if !svc.Ready(cd) {
		t.Fatal("cooldown did not recharge")
	}
}

func TestOwnedTimerDiesWithNode(t *testing.T) {
	svc, s, h := setup(t)
	tm, _ := svc.StartTimer(1, TimerOptions{Keep: true, Owner: h})
	s.Destroy(h)
	svc.Step(dt, false)
	if !svc.TimerDone(tm) || svc.Timers() != 0 {
		t.Fatal("owner death must drop the timer")
	}
}

func TestThousandTweensAllocateNothing(t *testing.T) {
	s := newScene(t)
	var nodes []scene.NodeHandle
	for range 1000 {
		h := s.Create(s.Root(), "n")
		scene.Add[probe](s, h)
		nodes = append(nodes, h)
	}
	s.Flush()
	svc := NewService(s, nil)
	for _, h := range nodes {
		if _, err := svc.ToVec2(h, "Probe.pos", mathx.Vec2{X: 1, Y: 1}, 0.5, Options{Legs: -1, PingPong: true, Ease: curve.OutBack}); err != nil {
			t.Fatal(err)
		}
	}
	for range 100 {
		svc.StartTimer(0.3, TimerOptions{Repeat: -1})
	}
	steps(svc, 4)
	if n := testing.AllocsPerRun(100, func() { svc.Step(dt, false) }); n != 0 {
		t.Fatalf("Step allocates %v", n)
	}
}
