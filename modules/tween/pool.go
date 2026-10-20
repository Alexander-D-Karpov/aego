package tween

type Handle struct {
	Index uint32
	Gen   uint32
}

func (h Handle) IsZero() bool {
	return h.Gen == 0
}

func (h Handle) Key() uint64 {
	return uint64(h.Gen)<<32 | uint64(h.Index)
}

func HandleOf(k uint64) Handle {
	return Handle{Index: uint32(k), Gen: uint32(k >> 32)}
}

type slot[T any] struct {
	gen  uint32
	live bool
	v    T
}

type pool[T any] struct {
	s    []slot[T]
	free []uint32
	n    int
}

func (p *pool[T]) alloc() (Handle, *T) {
	var i uint32
	if n := len(p.free); n > 0 {
		i = p.free[n-1]
		p.free = p.free[:n-1]
	} else {
		p.s = append(p.s, slot[T]{})
		i = uint32(len(p.s) - 1)
	}
	e := &p.s[i]
	e.gen++
	if e.gen == 0 {
		e.gen = 1
	}
	e.live = true
	p.n++
	return Handle{Index: i, Gen: e.gen}, &e.v
}

func (p *pool[T]) get(h Handle) *T {
	if h.Gen == 0 || int(h.Index) >= len(p.s) {
		return nil
	}
	e := &p.s[h.Index]
	if !e.live || e.gen != h.Gen {
		return nil
	}
	return &e.v
}

func (p *pool[T]) handle(i uint32) Handle {
	return Handle{Index: i, Gen: p.s[i].gen}
}

func (p *pool[T]) release(i uint32) {
	e := &p.s[i]
	if !e.live {
		return
	}
	e.live = false
	p.free = append(p.free, i)
	p.n--
}
