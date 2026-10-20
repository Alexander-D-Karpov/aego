package bind

import (
	"aego/script/abi"
	"aego/script/vm"
)

type Encoder[T any] struct {
	c codec[T]
}

func NewEncoder[T any](r *Registry) (Encoder[T], error) {
	c, err := codecOf[T](r)
	return Encoder[T]{c: c}, err
}

func (e Encoder[T]) Type() abi.TypeRef {
	return e.c.ref
}

func (e Encoder[T]) Slots() int {
	return e.c.slots
}

func (e Encoder[T]) Put(w []vm.Value, v T) {
	e.c.put(nil, w, v)
}

func (e Encoder[T]) Get(w []vm.Value) T {
	return e.c.get(nil, w)
}
