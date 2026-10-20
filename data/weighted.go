package data

import (
	"aego/core/rng"
	"aego/schema"
)

type Weighted[T any] struct {
	Items   []T       `aego:"items"`
	Weights []float32 `aego:"weights"`
}

func RegisterWeighted[T any](reg *schema.Registry, name string) error {
	_, err := schema.RegisterStruct[Weighted[T]](reg, schema.Name(name))
	return err
}

func (w *Weighted[T]) weight(i int) float32 {
	if i >= len(w.Weights) {
		return 1
	}
	return max(w.Weights[i], 0)
}

func (w *Weighted[T]) Pick(r *rng.Source) (*T, bool) {
	var total float32
	last := -1
	for i := range w.Items {
		if wt := w.weight(i); wt > 0 {
			total += wt
			last = i
		}
	}
	if last < 0 {
		return nil, false
	}
	x := r.Float32() * total
	for i := range w.Items {
		x -= w.weight(i)
		if x < 0 {
			return &w.Items[i], true
		}
	}
	return &w.Items[last], true
}
