package systems

import (
	"aego/contracts/scriptfields"
	"aego/contracts/scriptwait"
	"aego/engine"
	"aego/scene"
	"aego/script/runtime"
	"aego/script/vm"
)

type fields struct {
	w *engine.World
}

func runtimeIn(w *engine.World) *runtime.Runtime {
	rt, _ := engine.ServiceOf[*runtime.Runtime](w, runtime.ServiceID)
	return rt
}

func (f fields) ResolveField(node scene.NodeHandle, script, field string) (scriptfields.Ref, bool) {
	if rt := runtimeIn(f.w); rt != nil {
		return rt.ResolveField(node, script, field)
	}
	return scriptfields.Ref{}, false
}

func (f fields) LoadField(ref *scriptfields.Ref) (vm.Value, bool) {
	if rt := runtimeIn(f.w); rt != nil {
		return rt.LoadField(ref)
	}
	return vm.Value{}, false
}

func (f fields) StoreField(ref *scriptfields.Ref, v vm.Value) bool {
	rt := runtimeIn(f.w)
	return rt != nil && rt.StoreField(ref, v)
}

func FieldAccess(w *engine.World) scriptfields.Fields {
	return fields{w: w}
}

type waker struct {
	w *engine.World
}

func (k waker) Complete(typ, id uint64) {
	if rt := runtimeIn(k.w); rt != nil {
		rt.Complete(typ, id)
	}
}

func Waker(w *engine.World) scriptwait.Waker {
	return waker{w: w}
}
