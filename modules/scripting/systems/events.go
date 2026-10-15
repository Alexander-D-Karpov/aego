package systems

import (
	"aego/contracts/scriptevents"
	"aego/engine"
	"aego/scene"
	"aego/script/runtime"
	"aego/script/vm"
)

type eventsSystem struct{}

func (eventsSystem) Run(ctx *engine.Context) {
	if rt := runtimeOf(ctx); rt != nil {
		rt.DispatchEvents(ctx)
	}
}

type sink struct {
	w *engine.World
}

func (s sink) runtime() *runtime.Runtime {
	rt, _ := engine.ServiceOf[*runtime.Runtime](s.w, runtime.ServiceID)
	return rt
}

func (s sink) Emit(node scene.NodeHandle, ev scriptevents.EventID, args []vm.Value) {
	if rt := s.runtime(); rt != nil {
		rt.Emit(node, ev, args)
	}
}

func (s sink) ListensOn(node scene.NodeHandle, ev scriptevents.EventID) bool {
	rt := s.runtime()
	return rt != nil && rt.ListensOn(node, ev)
}

func (s sink) Flush() {
	if rt := s.runtime(); rt != nil {
		rt.Flush()
	}
}

func EventSink(w *engine.World) scriptevents.Sink {
	return sink{w: w}
}
