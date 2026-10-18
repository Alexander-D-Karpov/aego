package handle

import "aego/core/rewind"

func (a *Arena[T]) CopyTo(dst *Arena[T]) {
	dst.items = append(dst.items[:0], a.items...)
	dst.gens = append(dst.gens[:0], a.gens...)
	dst.alive = append(dst.alive[:0], a.alive...)
	dst.free = append(dst.free[:0], a.free...)
	dst.count = a.count
}

func (a *Arena[T]) RestoreFrom(src *Arena[T]) {
	src.CopyTo(a)
}

func (a *Arena[T]) Slots() int {
	return len(a.items)
}

func (a *Arena[T]) HashTo(h *rewind.Hasher) {
	h.Int(len(a.gens))
	for i, g := range a.gens {
		h.U32(g)
		h.Bool(a.alive[i])
	}
	h.Int(len(a.free))
	for _, f := range a.free {
		h.U32(f)
	}
	h.Int(a.count)
}
