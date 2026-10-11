package vm

import "time"

type Roots interface {
	MarkRoots(mk *Marker)
}

type Marker struct {
	h    *Heap
	work []uint32
}

func (mk *Marker) Mark(v Value) {
	o := mk.h.get(v.A)
	if o == nil || o.mark {
		return
	}
	o.mark = true
	if !o.str && o.mask != 0 {
		mk.work = append(mk.work, uint32(v.A>>32))
	}
}

func (mk *Marker) MarkSlots(vals []Value, slots []uint16) {
	for _, s := range slots {
		if int(s) < len(vals) {
			mk.Mark(vals[s])
		}
	}
}

func (m *Machine) Collect(r Roots) bool {
	if len(m.frames) != 0 {
		return false
	}
	start := time.Now()
	h := &m.heap
	mk := &m.marker
	mk.h = h
	if r != nil {
		r.MarkRoots(mk)
	}
	for i := range m.cos {
		c := &m.cos[i]
		if !c.live || c.state != CoSuspended {
			continue
		}
		for _, v := range c.regs {
			mk.Mark(v)
		}
	}
	for len(mk.work) > 0 {
		idx := mk.work[len(mk.work)-1]
		mk.work = mk.work[:len(mk.work)-1]
		o := &h.objs[idx]
		w := o.width
		for base := 0; base+w <= len(o.elems); base += w {
			for s := 0; s < w; s++ {
				if o.mask&(1<<uint(s)) != 0 {
					mk.Mark(o.elems[base+s])
				}
			}
		}
	}
	for i := range h.objs {
		o := &h.objs[i]
		if !o.live {
			continue
		}
		if o.mark {
			o.mark = false
			continue
		}
		h.bytes -= o.size
		o.live, o.s, o.elems, o.size = false, "", nil, 0
		h.free = append(h.free, uint32(i))
		h.live--
	}
	h.since = 0
	m.stats.GCRuns++
	m.stats.GCTime += time.Since(start)
	return true
}

func (m *Machine) MaybeCollect(r Roots) bool {
	if m.heap.since < m.opt.GCThreshold {
		return false
	}
	return m.Collect(r)
}
