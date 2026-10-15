package bind

import (
	"fmt"

	"aego/script/abi"
	"aego/script/vm"
)

type Out[T any] struct {
	m     *vm.Machine
	elems []vm.Value
	width int
	n     int
	c     *codec[T]
}

func (o Out[T]) Cap() int {
	return o.n
}

func (o Out[T]) Set(i int, v T) bool {
	if i < 0 || i >= o.n {
		return false
	}
	o.c.put(o.m, o.elems[i*o.width:(i+1)*o.width], v)
	return true
}

func (o Out[T]) Get(i int) (T, bool) {
	var zero T
	if i < 0 || i >= o.n {
		return zero, false
	}
	return o.c.get(o.m, o.elems[i*o.width:(i+1)*o.width]), true
}

func (Out[T]) outCodec(r *Registry) (any, error) {
	ce, err := codecOf[T](r)
	if err != nil {
		return nil, err
	}
	if !abi.Plain(ce.ref) {
		return nil, fmt.Errorf("Out of %s: the element must not live on the VM heap", ce.ref)
	}
	ec := &ce
	return codec[Out[T]]{
		ref:   abi.SliceOf(ce.ref),
		slots: 1,
		get: func(m *vm.Machine, w []vm.Value) Out[T] {
			elems, width, ok := m.SliceData(w[0])
			if !ok || width != ec.slots || width == 0 {
				return Out[T]{}
			}
			return Out[T]{m: m, elems: elems, width: width, n: len(elems) / width, c: ec}
		},
		put: func(*vm.Machine, []vm.Value, Out[T]) {},
	}, nil
}
