package handle

type Handle[T any] struct {
	Index uint32
	Gen   uint32
}

func (h Handle[T]) IsZero() bool {
	return h.Gen == 0
}

func Cast[U, T any](h Handle[T]) Handle[U] {
	return Handle[U](h)
}

type Arena[T any] struct {
	items  []T
	gens   []uint32
	alive  []bool
	free   []uint32
	count  int
	OnGrow func(oldCap, newCap int)
}

func NewArena[T any](capacity int) *Arena[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &Arena[T]{
		items: make([]T, 0, capacity),
		gens:  make([]uint32, 0, capacity),
		alive: make([]bool, 0, capacity),
		free:  make([]uint32, 0, capacity),
	}
}

func (a *Arena[T]) Alloc() (Handle[T], *T) {
	var idx uint32
	if n := len(a.free); n > 0 {
		idx = a.free[n-1]
		a.free = a.free[:n-1]
	} else {
		if len(a.items) == cap(a.items) && a.OnGrow != nil {
			a.OnGrow(cap(a.items), cap(a.items)*2)
		}
		var zero T
		idx = uint32(len(a.items))
		a.items = append(a.items, zero)
		a.gens = append(a.gens, 0)
		a.alive = append(a.alive, false)
	}
	gen := a.gens[idx] + 1
	if gen == 0 {
		gen = 1
	}
	a.gens[idx] = gen
	a.alive[idx] = true
	a.count++
	return Handle[T]{Index: idx, Gen: gen}, &a.items[idx]
}

func (a *Arena[T]) Valid(h Handle[T]) bool {
	return h.Gen != 0 &&
		int(h.Index) < len(a.items) &&
		a.alive[h.Index] &&
		a.gens[h.Index] == h.Gen
}

func (a *Arena[T]) Get(h Handle[T]) (*T, bool) {
	if !a.Valid(h) {
		return nil, false
	}
	return &a.items[h.Index], true
}

func (a *Arena[T]) Free(h Handle[T]) bool {
	if !a.Valid(h) {
		return false
	}
	var zero T
	a.items[h.Index] = zero
	a.alive[h.Index] = false
	a.free = append(a.free, h.Index)
	a.count--
	return true
}

func (a *Arena[T]) Len() int {
	return a.count
}

func (a *Arena[T]) Cap() int {
	return cap(a.items)
}

func (a *Arena[T]) Each(fn func(Handle[T], *T)) {
	for i := range a.items {
		if a.alive[i] {
			fn(Handle[T]{Index: uint32(i), Gen: a.gens[i]}, &a.items[i])
		}
	}
}

func (a *Arena[T]) Clear() {
	var zero T
	a.free = a.free[:0]
	n := len(a.items)
	for i := range a.items {
		a.items[i] = zero
		a.alive[i] = false
		a.free = append(a.free, uint32(n-1-i))
	}
	a.count = 0
}
