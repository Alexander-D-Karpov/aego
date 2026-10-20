package tween

import (
	"testing"

	"aego/contracts/signals"
	"aego/scene"
	"aego/script/vm"
)

type emitted struct {
	src signals.Source
	sig int
	arg vm.Value
}

type fakeHub struct {
	emits  []emitted
	ended  []signals.Source
	buf    [4]vm.Value
	probes map[uint64]func(uint64) bool
}

func (h *fakeHub) Wants(signals.Source) bool { return true }
func (h *fakeHub) Args(n int) []vm.Value     { return h.buf[:n] }
func (h *fakeHub) Emit(src signals.Source, sig int, args []vm.Value) {
	e := emitted{src: src, sig: sig}
	if len(args) > 0 {
		e.arg = args[0]
	}
	h.emits = append(h.emits, e)
}
func (h *fakeHub) End(src signals.Source) { h.ended = append(h.ended, src) }
func (h *fakeHub) Connect(signals.Source, int, signals.Handler) vm.Value {
	return vm.Value{A: 1}
}
func (h *fakeHub) ConnectScript(scene.NodeHandle, string, string, signals.Handler) vm.Value {
	return vm.Value{}
}
func (h *fakeHub) Disconnect(vm.Value)     {}
func (h *fakeHub) Connected(vm.Value) bool { return false }
func (h *fakeHub) Probe(typ uint64, alive func(uint64) bool) {
	if h.probes == nil {
		h.probes = map[uint64]func(uint64) bool{}
	}
	h.probes[typ] = alive
}
func (h *fakeHub) HandleAlive(typ, key uint64) bool {
	p := h.probes[typ]
	return p == nil || p(key)
}

func TestCompletedSignal(t *testing.T) {
	svc, _, h := setup(t)
	hub := &fakeHub{}
	svc.hub = hub
	a, _ := svc.ToFloat(h, "Probe.value", 1, 0.25, Options{})
	steps(svc, 4)
	b, _ := svc.ToFloat(h, "Probe.value", 2, 0.25, Options{})
	steps(svc, 16)
	c, _ := svc.ToFloat(h, "Probe.value", 3, 1, Options{})
	svc.Stop(c)
	want := []struct {
		key      uint64
		finished bool
	}{{a.Key(), false}, {b.Key(), true}, {c.Key(), false}}
	if len(hub.emits) != len(want) || len(hub.ended) != len(want) {
		t.Fatalf("emits %d ended %d", len(hub.emits), len(hub.ended))
	}
	for i, w := range want {
		e := hub.emits[i]
		if e.src != handleSource(typeTween, w.key) || e.sig != sigCompleted || e.arg.Bool() != w.finished {
			t.Fatalf("emit %d: %+v, want key %d finished %v", i, e, w.key, w.finished)
		}
		if hub.ended[i] != e.src {
			t.Fatalf("end %d does not follow its emit", i)
		}
	}
}

func TestFiredSignalAndEnd(t *testing.T) {
	svc, _, _ := setup(t)
	hub := &fakeHub{}
	svc.hub = hub
	tm, _ := svc.StartTimer(0.25, TimerOptions{Repeat: 2})
	steps(svc, 32)
	if len(hub.emits) != 2 || hub.emits[0].arg.Int() != 1 || hub.emits[1].arg.Int() != 2 {
		t.Fatalf("fired %+v", hub.emits)
	}
	if len(hub.ended) != 1 || hub.ended[0] != handleSource(typeTimer, tm.Key()) {
		t.Fatalf("a one-shot timer must end its handle once, got %v", hub.ended)
	}
	cd, _ := svc.Cooldown(0.25, TimerOptions{})
	svc.Use(cd)
	steps(svc, 16)
	if len(hub.ended) != 1 {
		t.Fatal("a kept timer must not end its handle when it fires")
	}
	svc.Cancel(cd)
	if len(hub.ended) != 2 {
		t.Fatal("Cancel must end the handle")
	}
}

func TestProbes(t *testing.T) {
	svc, _, h := setup(t)
	hub := &fakeHub{}
	svc.hub = hub
	hub.Probe(typeTween, svc.tweenAlive)
	tw, _ := svc.ToFloat(h, "Probe.value", 1, 1, Options{})
	if !hub.HandleAlive(typeTween, tw.Key()) {
		t.Fatal("a running tween is alive")
	}
	svc.Stop(tw)
	if hub.HandleAlive(typeTween, tw.Key()) {
		t.Fatal("a stopped tween is not alive")
	}
}
