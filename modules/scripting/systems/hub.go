package systems

import (
	"aego/contracts/signals"
	"aego/engine"
	"aego/scene"
	"aego/script/signal"
	"aego/script/vm"
)

type hub struct {
	w      *engine.World
	probes map[uint64]func(uint64) bool
	buf    []vm.Value
}

func Hub(w *engine.World) signals.Hub {
	return &hub{w: w, probes: map[uint64]func(uint64) bool{}}
}

func (h *hub) Wants(src signals.Source) bool {
	rt := runtimeIn(h.w)
	return rt != nil && rt.WantsSignal(signal.Source(src))
}

func (h *hub) Args(n int) []vm.Value {
	if rt := runtimeIn(h.w); rt != nil {
		return rt.SignalArgs(n)
	}
	if cap(h.buf) < n {
		h.buf = make([]vm.Value, n)
	}
	return h.buf[:n]
}

func (h *hub) Emit(src signals.Source, sig int, args []vm.Value) {
	if rt := runtimeIn(h.w); rt != nil {
		rt.SetRecorder(h.w.Recorder)
		rt.EmitHost(signal.Source(src), sig, args)
	}
}

func (h *hub) End(src signals.Source) {
	if rt := runtimeIn(h.w); rt != nil {
		rt.EndSource(signal.Source(src))
	}
}

func (h *hub) Connect(src signals.Source, sig int, fn signals.Handler) vm.Value {
	rt := runtimeIn(h.w)
	if rt == nil || fn == nil || (src.Kind == signals.KindHandle && !h.HandleAlive(src.B, src.A)) {
		return vm.Value{}
	}
	return rt.ConnectHost(signal.Source(src), sig, signal.Handler(fn))
}

func (h *hub) ConnectScript(node scene.NodeHandle, script, name string, fn signals.Handler) vm.Value {
	rt := runtimeIn(h.w)
	if rt == nil || fn == nil {
		return vm.Value{}
	}
	return rt.ConnectScriptHost(node, script, name, signal.Handler(fn))
}

func (h *hub) Disconnect(c vm.Value) {
	if rt := runtimeIn(h.w); rt != nil && !c.IsZero() {
		rt.DisconnectSignal(c)
	}
}

func (h *hub) Connected(c vm.Value) bool {
	rt := runtimeIn(h.w)
	return rt != nil && !c.IsZero() && rt.SignalConnected(c)
}

func (h *hub) Probe(typ uint64, alive func(key uint64) bool) {
	if alive == nil {
		delete(h.probes, typ)
		return
	}
	h.probes[typ] = alive
}

func (h *hub) HandleAlive(typ, key uint64) bool {
	p := h.probes[typ]
	return p == nil || p(key)
}
