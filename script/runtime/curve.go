package runtime

import (
	"unsafe"

	"aego/core/curve"
	"aego/schema"
	"aego/script/bind"
	"aego/script/vm"
)

var _ bind.CurveSource = (*Runtime)(nil)

func (r *Runtime) CurveOf(v vm.Value) (*curve.Curve, error) {
	p, fd, err := r.component(vm.Value{A: v.A, B: uint64(uint32(v.B))}, int(v.B>>32))
	if err != nil {
		return nil, err
	}
	if fd.Kind != schema.KindCurve {
		return nil, errBadComponent
	}
	return (*curve.Curve)(unsafe.Add(p, fd.Offset)), nil
}

func (r *Runtime) Complete(typ, id uint64) {
	if r.co != nil {
		r.co.Complete(typ, id)
	}
}
