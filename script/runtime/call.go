package runtime

import (
	"aego/contracts/scriptcall"
	"aego/scene"
	"aego/script/vm"
)

// Method finds a plain method of a running script on the node: never a lifecycle method, an event handler or a coroutine.
func (r *Runtime) Method(h scene.NodeHandle, name string, params int) (scriptcall.Ref, bool) {
	for _, iid := range r.byNode[h] {
		in := &r.inst[iid-1]
		if in.state != Running || in.base < 0 || in.typ == nil {
			continue
		}
		i := in.typ.methodIndex(name)
		if i < 0 {
			continue
		}
		f := in.typ.methods[i]
		if f.Coroutine || f.Params != params || in.typ.lifecycle(f) {
			continue
		}
		return scriptcall.Ref{Node: h, Instance: iid, Index: int32(i), Gen: r.Generation()}, true
	}
	return scriptcall.Ref{}, false
}

// CallMethod calls the method of ref and reports whether its instance still runs.
func (r *Runtime) CallMethod(ref *scriptcall.Ref, args []vm.Value) bool {
	if ref.Gen != r.Generation() || ref.Instance == vm.NoInstance || int(ref.Instance) > len(r.inst) {
		return false
	}
	in := &r.inst[ref.Instance-1]
	if in.state != Running || in.node != ref.Node || in.typ == nil || int(ref.Index) >= len(in.typ.methods) {
		return false
	}
	r.call(ref.Instance, in.typ.methods[ref.Index], args)
	return in.state == Running
}
