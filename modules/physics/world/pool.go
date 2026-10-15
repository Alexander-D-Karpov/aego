package world

import "aego/core/handle"

type pool[T any] struct {
	items []T
	gens  []uint32
	alive []bool
	free  []uint32
	count int
}

func newPool[T any](capacity int) pool[T] {
	return pool[T]{
		items: make([]T, 0, capacity),
		gens:  make([]uint32, 0, capacity),
		alive: make([]bool, 0, capacity),
		free:  make([]uint32, 0, capacity),
	}
}

func (p *pool[T]) alloc() (handle.Handle[T], *T) {
	var idx uint32
	if n := len(p.free); n > 0 {
		idx = p.free[n-1]
		p.free = p.free[:n-1]
	} else {
		var zero T
		idx = uint32(len(p.items))
		p.items = append(p.items, zero)
		p.gens = append(p.gens, 0)
		p.alive = append(p.alive, false)
	}
	gen := p.gens[idx] + 1
	if gen == 0 {
		gen = 1
	}
	p.gens[idx] = gen
	p.alive[idx] = true
	p.count++
	return handle.Handle[T]{Index: idx, Gen: gen}, &p.items[idx]
}

func (p *pool[T]) valid(h handle.Handle[T]) bool {
	return h.Gen != 0 && int(h.Index) < len(p.items) && p.alive[h.Index] && p.gens[h.Index] == h.Gen
}

func (p *pool[T]) get(h handle.Handle[T]) (*T, bool) {
	if !p.valid(h) {
		return nil, false
	}
	return &p.items[h.Index], true
}

func (p *pool[T]) release(h handle.Handle[T]) bool {
	if !p.valid(h) {
		return false
	}
	var zero T
	p.items[h.Index] = zero
	p.alive[h.Index] = false
	p.free = append(p.free, h.Index)
	p.count--
	return true
}

func (p *pool[T]) handleAt(i int) handle.Handle[T] {
	return handle.Handle[T]{Index: uint32(i), Gen: p.gens[i]}
}

func (p *pool[T]) clone(cp func(dst, src *T)) pool[T] {
	out := pool[T]{
		items: make([]T, len(p.items), cap(p.items)),
		gens:  append(make([]uint32, 0, cap(p.gens)), p.gens...),
		alive: append(make([]bool, 0, cap(p.alive)), p.alive...),
		free:  append(make([]uint32, 0, cap(p.free)), p.free...),
		count: p.count,
	}
	for i := range p.items {
		if cp != nil {
			cp(&out.items[i], &p.items[i])
		} else {
			out.items[i] = p.items[i]
		}
	}
	return out
}
