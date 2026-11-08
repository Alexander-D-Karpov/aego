package vm

import "strconv"

const staticBit = uint64(1) << 63

type object struct {
	gen   uint32
	live  bool
	mark  bool
	str   bool
	s     string
	elems []Value
	width int
	mask  uint32
	size  int
}

type heapFull struct {
	limit int
}

func (f heapFull) Error() string {
	return "the script heap is over its " + strconv.Itoa(f.limit>>20) + " MB limit"
}

type Heap struct {
	objs []object
	free []uint32
	live int
	// Live slice objects, maintained by allocation, collection and restore.
	// When zero, every object's elems is nil and snapshots need no slice scan.
	sliceCount int
	bytes      int
	since      int
	limit      int
	armed      int
}

func (h *Heap) charge(size int) {
	if h.armed > 0 && h.limit > 0 && h.bytes+size > h.limit {
		panic(heapFull{limit: h.limit})
	}
}

func heapHandle(idx, gen uint32) uint64 {
	return uint64(idx)<<32 | uint64(gen)
}

func (h *Heap) alloc(size int) (uint32, *object) {
	h.charge(size)
	var idx uint32
	if n := len(h.free); n > 0 {
		idx = h.free[n-1]
		h.free = h.free[:n-1]
	} else {
		h.objs = append(h.objs, object{})
		idx = uint32(len(h.objs) - 1)
	}
	o := &h.objs[idx]
	o.gen++
	if o.gen == 0 {
		o.gen = 1
	}
	o.live, o.mark, o.size = true, false, size
	h.live++
	h.bytes += size
	h.since += size
	return idx, o
}

func (h *Heap) get(a uint64) *object {
	if a == 0 || a&staticBit != 0 {
		return nil
	}
	idx := uint32(a >> 32)
	if int(idx) >= len(h.objs) {
		return nil
	}
	o := &h.objs[idx]
	if !o.live || o.gen != uint32(a) {
		return nil
	}
	return o
}

func (h *Heap) newString(s string) Value {
	idx, o := h.alloc(len(s) + 32)
	o.str, o.s, o.elems, o.width, o.mask = true, s, nil, 0, 0
	return Value{A: heapHandle(idx, o.gen)}
}

func (h *Heap) newSlice(width int, mask uint32, capacity int) Value {
	idx, o := h.alloc(capacity*width*16 + 48)
	o.str, o.s = false, ""
	o.elems = make([]Value, 0, capacity*width)
	o.width, o.mask = width, mask
	h.sliceCount++
	return Value{A: heapHandle(idx, o.gen)}
}

func (h *Heap) grow(o *object, delta int) {
	h.charge(delta)
	o.size += delta
	h.bytes += delta
	h.since += delta
}
