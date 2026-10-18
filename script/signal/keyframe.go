package signal

import (
	"slices"
	"unsafe"

	"aego/core/rewind"
)

type srcKey struct {
	src Source
	n   int32
}

type State struct {
	conns  []entry
	free   []uint32
	srcs   []srcKey
	idx    []uint32
	n      int
	hosted int
}

func (s *State) Size() int {
	return len(s.conns)*int(unsafe.Sizeof(entry{})) + 4*(len(s.free)+len(s.idx)) + len(s.srcs)*int(unsafe.Sizeof(srcKey{}))
}

func (r *Registry) Capture(dst *State) {
	dst.conns = append(dst.conns[:0], r.conns...)
	dst.free = append(dst.free[:0], r.free...)
	dst.srcs, dst.idx = dst.srcs[:0], dst.idx[:0]
	for src, list := range r.bySrc {
		dst.srcs = append(dst.srcs, srcKey{src: src, n: int32(len(list))})
		dst.idx = append(dst.idx, list...)
	}
	dst.n, dst.hosted = r.n, r.hosted
}

func (r *Registry) Restore(src *State) {
	r.conns = append(r.conns[:0], src.conns...)
	r.free = append(r.free[:0], src.free...)
	clear(r.bySrc)
	buf := slices.Clone(src.idx)
	off := 0
	for _, s := range src.srcs {
		n := int(s.n)
		r.bySrc[s.src] = buf[off : off+n : off+n]
		off += n
	}
	r.scratch, r.drop = r.scratch[:0], r.drop[:0]
	r.depth, r.pending = 0, false
	r.n, r.hosted = src.n, src.hosted
}

func (r *Registry) Hash(h *rewind.Hasher) {
	h.String("signals")
	h.Int(len(r.conns))
	for i := range r.conns {
		e := &r.conns[i]
		h.U32(e.gen)
		h.Bool(e.live)
		h.Bool(e.dead)
		if !e.live {
			continue
		}
		c := &e.c
		h.U64(uint64(c.Src.Kind))
		h.U64(c.Src.A)
		h.U64(c.Src.B)
		h.Int(c.Sig)
		h.U32(uint32(c.Dst))
		h.Int(c.Method)
		h.String(c.Signal)
		h.String(c.Handler)
		h.Bool(c.Go != nil)
	}
	h.Int(len(r.free))
	for _, f := range r.free {
		h.U32(f)
	}
	h.Int(r.n)
	h.Int(r.hosted)
}
