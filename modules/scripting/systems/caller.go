package systems

import (
	"aego/contracts/scriptcall"
	"aego/engine"
	"aego/scene"
	"aego/script/vm"
)

type caller struct {
	w *engine.World
}

func Caller(w *engine.World) scriptcall.Caller {
	return caller{w: w}
}

func (c caller) Method(h scene.NodeHandle, name string, params int) (scriptcall.Ref, bool) {
	if rt := runtimeIn(c.w); rt != nil {
		return rt.Method(h, name, params)
	}
	return scriptcall.Ref{}, false
}

func (c caller) Call(ref *scriptcall.Ref, args []vm.Value) bool {
	rt := runtimeIn(c.w)
	return rt != nil && rt.CallMethod(ref, args)
}

func (c caller) Generation() uint64 {
	if rt := runtimeIn(c.w); rt != nil {
		return rt.Generation()
	}
	return 0
}
