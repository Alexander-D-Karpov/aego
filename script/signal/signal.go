package signal

import (
	"aego/script/abi"
	"aego/script/vm"
)

type Conn struct {
	Src     vm.InstanceID
	Sig     int
	Dst     vm.InstanceID
	Method  int
	Signal  string
	Handler string
	Sign    abi.Signature
}

type Deliver func(dst vm.InstanceID, method int, args []vm.Value) (bool, error)

type Handle struct {
	Index uint32
	Gen   uint32
}

func Value(h Handle) vm.Value {
	if h.Gen == 0 {
		return vm.Value{}
	}
	return vm.Value{A: uint64(h.Gen)<<32 | uint64(h.Index)}
}

func Of(v vm.Value) Handle {
	return Handle{Index: uint32(v.A), Gen: uint32(v.A >> 32)}
}

type entry struct {
	gen  uint32
	live bool
	dead bool
	c    Conn
}

type snap struct {
	idx uint32
	gen uint32
}

type Registry struct {
	conns   []entry
	free    []uint32
	bySrc   map[vm.InstanceID][]uint32
	scratch []snap
	depth   int
	pending bool
	n       int
}

func New() *Registry {
	return &Registry{bySrc: map[vm.InstanceID][]uint32{}}
}

func (r *Registry) get(h Handle) *entry {
	if h.Gen == 0 || int(h.Index) >= len(r.conns) {
		return nil
	}
	e := &r.conns[h.Index]
	if !e.live || e.gen != h.Gen {
		return nil
	}
	return e
}

func (r *Registry) Len() int {
	return r.n
}

func (r *Registry) Connect(c Conn) Handle {
	var i uint32
	if n := len(r.free); n > 0 {
		i = r.free[n-1]
		r.free = r.free[:n-1]
	} else {
		r.conns = append(r.conns, entry{})
		i = uint32(len(r.conns) - 1)
	}
	e := &r.conns[i]
	e.gen++
	if e.gen == 0 {
		e.gen = 1
	}
	e.live, e.dead, e.c = true, false, c
	r.bySrc[c.Src] = append(r.bySrc[c.Src], i)
	r.n++
	return Handle{Index: i, Gen: e.gen}
}

func (r *Registry) Connected(h Handle) bool {
	e := r.get(h)
	return e != nil && !e.dead
}

func (r *Registry) Disconnect(h Handle) {
	if e := r.get(h); e != nil {
		r.kill(h.Index)
	}
}

func (r *Registry) kill(i uint32) {
	e := &r.conns[i]
	if !e.dead {
		e.dead = true
		r.n--
	}
	if r.depth > 0 {
		r.pending = true
		return
	}
	r.remove(i)
}

func (r *Registry) remove(i uint32) {
	e := &r.conns[i]
	src := e.c.Src
	list := r.bySrc[src]
	for k, j := range list {
		if j == i {
			list = append(list[:k], list[k+1:]...)
			break
		}
	}
	if len(list) == 0 {
		delete(r.bySrc, src)
	} else {
		r.bySrc[src] = list
	}
	e.live, e.dead, e.c = false, false, Conn{}
	r.free = append(r.free, i)
}

func (r *Registry) compact() {
	r.pending = false
	for i := range r.conns {
		if e := &r.conns[i]; e.live && e.dead {
			r.remove(uint32(i))
		}
	}
}

func (r *Registry) Emit(src vm.InstanceID, sig int, args []vm.Value, deliver Deliver) error {
	list := r.bySrc[src]
	if len(list) == 0 {
		return nil
	}
	base := len(r.scratch)
	for _, i := range list {
		if e := &r.conns[i]; !e.dead && e.c.Sig == sig {
			r.scratch = append(r.scratch, snap{idx: i, gen: e.gen})
		}
	}
	end := len(r.scratch)
	r.depth++
	var err error
	for k := base; k < end; k++ {
		s := r.scratch[k]
		e := &r.conns[s.idx]
		if !e.live || e.gen != s.gen {
			continue
		}
		dst, method := e.c.Dst, e.c.Method
		ok, derr := deliver(dst, method, args)
		if !ok {
			if e := &r.conns[s.idx]; e.live && e.gen == s.gen {
				r.kill(s.idx)
			}
		}
		if derr != nil {
			err = derr
			break
		}
	}
	r.scratch = r.scratch[:base]
	r.depth--
	if r.depth == 0 && r.pending {
		r.compact()
	}
	return err
}

func (r *Registry) DropInstance(id vm.InstanceID) {
	if id == vm.NoInstance {
		return
	}
	for i := range r.conns {
		e := &r.conns[i]
		if e.live && !e.dead && (e.c.Src == id || e.c.Dst == id) {
			r.kill(uint32(i))
		}
	}
}

func (r *Registry) Rebind(fn func(*Conn) bool) int {
	n := 0
	for i := range r.conns {
		e := &r.conns[i]
		if !e.live || e.dead {
			continue
		}
		if !fn(&e.c) {
			r.kill(uint32(i))
			n++
		}
	}
	return n
}

func (r *Registry) Reset() {
	for i := range r.conns {
		e := &r.conns[i]
		if e.live {
			e.live, e.dead, e.c = false, false, Conn{}
			r.free = append(r.free, uint32(i))
		}
	}
	clear(r.bySrc)
	r.scratch = r.scratch[:0]
	r.depth, r.pending, r.n = 0, false, 0
}
