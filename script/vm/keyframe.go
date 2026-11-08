package vm

import (
	"unsafe"

	"aego/core/rewind"
)

type State struct {
	objs       []object
	elems      []Value
	lens       []int32
	caps       []int32
	free       []uint32
	live       int
	sliceCount int
	bytes      int
	since      int
	cos        []coroutine
	regs       []Value
	rlens      []int32
	cofree     []uint32
	colive     int

	hostStrings [hostStringCacheSize]Value
	hostNext    uint8
}

func (s *State) Size() int {
	v := int(unsafe.Sizeof(Value{}))
	return len(s.objs)*int(unsafe.Sizeof(object{})) + len(s.elems)*v + 8*len(s.lens) + 4*len(s.free) +
		len(s.cos)*int(unsafe.Sizeof(coroutine{})) + len(s.regs)*v + 4*len(s.rlens) + 4*len(s.cofree) +
		int(unsafe.Sizeof(s.hostStrings)) + int(unsafe.Sizeof(s.hostNext))
}

func (m *Machine) Capture(dst *State) {
	h := &m.heap
	dst.hostStrings, dst.hostNext = m.hostStrings, m.hostNext
	dst.objs = append(dst.objs[:0], h.objs...)
	dst.elems, dst.lens, dst.caps = dst.elems[:0], dst.lens[:0], dst.caps[:0]
	dst.sliceCount = h.sliceCount
	// String-only heaps have no mutable slice payloads to detach. In particular,
	// dead objects have nil elems because collection clears them before reuse.
	if h.sliceCount != 0 {
		for i := range dst.objs {
			o := &dst.objs[i]
			dst.lens = append(dst.lens, int32(len(o.elems)))
			dst.caps = append(dst.caps, int32(cap(o.elems)))
			dst.elems = append(dst.elems, o.elems...)
			o.elems = nil
		}
	}
	dst.free = append(dst.free[:0], h.free...)
	dst.live, dst.bytes, dst.since = h.live, h.bytes, h.since
	dst.cos = append(dst.cos[:0], m.cos...)
	dst.regs, dst.rlens = dst.regs[:0], dst.rlens[:0]
	for i := range dst.cos {
		c := &dst.cos[i]
		dst.rlens = append(dst.rlens, int32(len(c.regs)))
		dst.regs = append(dst.regs, c.regs...)
		c.regs = nil
	}
	dst.cofree = append(dst.cofree[:0], m.cofree...)
	dst.colive = m.colive
}

func (m *Machine) Restore(src *State) {
	h := &m.heap
	m.hostStrings, m.hostNext = src.hostStrings, src.hostNext
	n := len(src.objs)
	objs := h.objs
	if cap(objs) < n {
		objs = make([]object, n)
		copy(objs, h.objs)
	} else {
		objs = objs[:n]
	}
	off := 0
	if src.sliceCount == 0 {
		copy(objs, src.objs)
	} else {
		for i := range src.objs {
			prev := objs[i].elems
			objs[i] = src.objs[i]
			ln, c := int(src.lens[i]), int(src.caps[i])
			if c > 0 {
				e := prev
				if cap(e) != c {
					e = make([]Value, ln, c)
				} else {
					e = e[:ln]
				}
				copy(e, src.elems[off:off+ln])
				objs[i].elems = e
			}
			off += ln
		}
	}
	clear(objs[n:cap(objs)])
	h.objs = objs
	h.sliceCount = src.sliceCount
	h.free = append(h.free[:0], src.free...)
	h.live, h.bytes, h.since = src.live, src.bytes, src.since
	cn := len(src.cos)
	cos := m.cos
	if cap(cos) < cn {
		cos = make([]coroutine, cn)
		copy(cos, m.cos)
	} else {
		cos = cos[:cn]
	}
	off = 0
	for i := range src.cos {
		prev := cos[i].regs
		cos[i] = src.cos[i]
		ln := int(src.rlens[i])
		r := prev
		if cap(r) < ln {
			r = make([]Value, ln)
		} else {
			r = r[:ln]
		}
		copy(r, src.regs[off:off+ln])
		cos[i].regs = r
		off += ln
	}
	m.cos = cos
	m.cofree = append(m.cofree[:0], src.cofree...)
	m.colive = src.colive
}

func HashValues(h *rewind.Hasher, vs []Value) {
	h.Int(len(vs))
	for _, v := range vs {
		h.U64(v.A)
		h.U64(v.B)
	}
}

func (m *Machine) Hash(h *rewind.Hasher) {
	hp := &m.heap
	h.String("vm")
	HashValues(h, m.hostStrings[:])
	h.U32(uint32(m.hostNext))
	h.Int(len(hp.objs))
	for i := range hp.objs {
		o := &hp.objs[i]
		h.U32(o.gen)
		h.Bool(o.live)
		if !o.live {
			continue
		}
		h.Bool(o.str)
		h.String(o.s)
		h.Int(o.width)
		h.U32(o.mask)
		h.Int(o.size)
		h.Int(cap(o.elems))
		HashValues(h, o.elems)
	}
	h.Int(len(hp.free))
	for _, f := range hp.free {
		h.U32(f)
	}
	h.Int(hp.live)
	h.Int(hp.bytes)
	h.Int(hp.since)
	h.Int(len(m.cos))
	for i := range m.cos {
		c := &m.cos[i]
		h.U32(c.gen)
		h.Bool(c.live)
		if !c.live {
			continue
		}
		h.Bool(c.canceled)
		if c.fn != nil {
			h.String(c.fn.Name)
		}
		h.Int(c.pc)
		h.U32(uint32(c.owner))
		h.U64(uint64(c.state))
		HashValues(h, c.regs)
	}
	h.Int(len(m.cofree))
	for _, f := range m.cofree {
		h.U32(f)
	}
	h.Int(m.colive)
	h.Int(m.opt.MaxCallDepth)
	h.U64(m.opt.Budget)
	h.Int(m.opt.GCThreshold)
}
