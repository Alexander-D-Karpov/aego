package bind

import (
	"aego/core/curve"
	"aego/script/abi"
	"aego/script/vm"
)

type CurveSource interface {
	CurveOf(v vm.Value) (*curve.Curve, error)
}

type Curve struct {
	v vm.Value
	m *vm.Machine
}

var errNilCurve = &vm.Fault{Code: vm.CodeNil, Message: "curve reference is nil or its owner is gone"}

func (c Curve) IsZero() bool {
	return c.v.IsZero()
}

func (c Curve) Resolve() (*curve.Curve, error) {
	if c.v.IsZero() || c.m == nil {
		return nil, errNilCurve
	}
	src, ok := c.m.Comps.(CurveSource)
	if !ok {
		return nil, errNilCurve
	}
	return src.CurveOf(c.v)
}

func curveCodec[T any](c *codec[T]) {
	c.ref = abi.T(abi.Curve)
	c.get = func(m *vm.Machine, w []vm.Value) T { return cast[T](Curve{v: w[0], m: m}) }
	c.put = func(_ *vm.Machine, w []vm.Value, v T) { w[0] = cast[Curve](v).v }
}
