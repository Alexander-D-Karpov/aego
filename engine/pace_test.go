package engine

import (
	"testing"
	"time"

	"aego/core/log"
	"aego/platform"
	"aego/platform/headless"
)

type paceProbe struct {
	testApp
	clock *resumableClock
	calls int
}

func (a *paceProbe) PaceFrame() {
	a.calls++
	tr := a.e.Trace()
	s := tr.Begin(tr.Zone(ZoneIdleWait))
	a.clock.now += 0.070
	s.End()
	tr.AddPaceWait(70*time.Millisecond, true)
}

func TestPacerIsOutsideRenderAndNotCalledWhenMinimized(t *testing.T) {
	p := &resumableClock{Platform: headless.New(headless.Options{})}
	cfg := Config{Headless: true, Trace: true, ResRoot: t.TempDir(), UserRoot: t.TempDir()}
	e, err := NewBuilder(cfg).WithLogger(log.Nop()).WithPlatform(p).Build()
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	a := &paceProbe{clock: p}
	if !e.Step(a) {
		t.Fatal("first step failed")
	}
	f := e.Trace().Frame(0)
	var renderEnd, swapStart, waitStart, waitEnd uint32
	for _, ev := range f.Events {
		switch e.Trace().Name(ev.Zone) {
		case SystemAppRender:
			renderEnd = ev.Start + ev.Dur
		case "engine.swap":
			swapStart = ev.Start
		case ZoneIdleWait:
			if ev.Depth != 0 {
				t.Fatalf("wait is nested in a render zone: %+v", ev)
			}
			waitStart = ev.Start
			waitEnd = ev.Start + ev.Dur
		}
	}
	if a.calls != 1 || a.renders != 1 || renderEnd == 0 || swapStart == 0 || waitStart < renderEnd || waitEnd > swapStart {
		t.Fatalf("calls=%d renders=%d render end=%d wait=%d..%d swap start=%d", a.calls, a.renders, renderEnd, waitStart, waitEnd, swapStart)
	}
	if e.Stats().RenderTime != 0 || f.PaceWaitNS != int64(70*time.Millisecond) || f.IdleWaitNS != int64(70*time.Millisecond) {
		t.Fatalf("render=%v wait=%d idle=%d", e.Stats().RenderTime, f.PaceWaitNS, f.IdleWaitNS)
	}
	p.Push(platform.Event{Kind: platform.EvWindowMinimized})
	e.Step(a)
	if a.calls != 1 || a.renders != 1 {
		t.Fatalf("pacing or rendering ran while minimized: calls=%d renders=%d", a.calls, a.renders)
	}
}
