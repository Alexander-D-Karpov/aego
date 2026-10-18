package coroutine

import (
	"cmp"
	"slices"
	"unsafe"

	"aego/core/rewind"
	"aego/script/vm"
)

type awaitKey struct {
	h vm.CoHandle
	n int32
}

type handleKey struct {
	k [2]uint64
	n int32
}

type State struct {
	recs     []rec
	await    []awaitKey
	waits    []uint32
	hands    []handleKey
	hwaits   []uint32
	frames   []uint32
	timed    []uint32
	fixed    []uint32
	unscaled []uint32
	woken    []item
	hwoken   []item
	live     int
	waiters  int
	seq      uint64
	now      float64
	real     float64
	frame    uint64
	uframe   uint64
	fixedN   uint64
}

func (s *State) Size() int {
	return len(s.recs)*int(unsafe.Sizeof(rec{})) + len(s.await)*int(unsafe.Sizeof(awaitKey{})) +
		len(s.hands)*int(unsafe.Sizeof(handleKey{})) + 4*(len(s.waits)+len(s.hwaits)+len(s.frames)+len(s.timed)+len(s.fixed)+len(s.unscaled)) +
		(len(s.woken)+len(s.hwoken))*int(unsafe.Sizeof(item{}))
}

func (s *Scheduler) Capture(dst *State) {
	dst.recs = append(dst.recs[:0], s.recs...)
	dst.await, dst.waits = dst.await[:0], dst.waits[:0]
	for h, ws := range s.awaiting {
		dst.await = append(dst.await, awaitKey{h: h, n: int32(len(ws))})
		dst.waits = append(dst.waits, ws...)
	}
	dst.hands, dst.hwaits = dst.hands[:0], dst.hwaits[:0]
	for k, ws := range s.handles {
		dst.hands = append(dst.hands, handleKey{k: k, n: int32(len(ws))})
		dst.hwaits = append(dst.hwaits, ws...)
	}
	dst.frames = append(dst.frames[:0], s.frames.heap...)
	dst.timed = append(dst.timed[:0], s.timed.heap...)
	dst.fixed = append(dst.fixed[:0], s.fixed.heap...)
	dst.unscaled = append(dst.unscaled[:0], s.unscaled.heap...)
	dst.woken = append(dst.woken[:0], s.woken...)
	dst.hwoken = append(dst.hwoken[:0], s.hwoken...)
	dst.live, dst.waiters, dst.seq = s.live, s.waiters, s.seq
	dst.now, dst.real = s.now, s.real
	dst.frame, dst.uframe, dst.fixedN = s.frame, s.uframe, s.fixedN
}

func (s *Scheduler) Restore(src *State) {
	s.recs = append(s.recs[:0], src.recs...)
	clear(s.awaiting)
	buf := slices.Clone(src.waits)
	off := 0
	for _, a := range src.await {
		n := int(a.n)
		s.awaiting[a.h] = buf[off : off+n : off+n]
		off += n
	}
	clear(s.handles)
	hbuf := slices.Clone(src.hwaits)
	off = 0
	for _, a := range src.hands {
		n := int(a.n)
		s.handles[a.k] = hbuf[off : off+n : off+n]
		off += n
	}
	s.frames.heap = append(s.frames.heap[:0], src.frames...)
	s.timed.heap = append(s.timed.heap[:0], src.timed...)
	s.fixed.heap = append(s.fixed.heap[:0], src.fixed...)
	s.unscaled.heap = append(s.unscaled.heap[:0], src.unscaled...)
	s.woken = append(s.woken[:0], src.woken...)
	s.hwoken = append(s.hwoken[:0], src.hwoken...)
	s.run, s.frun, s.urun, s.later = s.run[:0], s.frun[:0], s.urun[:0], s.later[:0]
	s.live, s.waiters, s.seq = src.live, src.waiters, src.seq
	s.now, s.real = src.now, src.real
	s.frame, s.uframe, s.fixedN = src.frame, src.uframe, src.fixedN
	s.inFrame, s.inFixed, s.inU = false, false, false
}

func hashItems(h *rewind.Hasher, list []item) {
	h.Int(len(list))
	for _, it := range list {
		h.U32(it.h.Index)
		h.U32(it.h.Gen)
		h.U64(it.seq)
	}
}

func hashWaits(h *rewind.Hasher, ws []uint32) {
	h.Int(len(ws))
	for _, w := range ws {
		h.U32(w)
	}
}

func (s *Scheduler) Hash(h *rewind.Hasher) {
	h.String("co")
	h.Int(len(s.recs))
	for i := range s.recs {
		r := &s.recs[i]
		h.U32(r.gen)
		h.Bool(r.live)
		if !r.live {
			continue
		}
		h.U32(uint32(r.owner))
		h.U64(r.seq)
		h.U64(uint64(r.where))
		h.Int(r.pos)
		h.F64(r.key)
		h.U64(r.frame)
		h.U32(r.target.Index)
		h.U32(r.target.Gen)
		h.U64(r.htyp)
		h.U64(r.hid)
	}
	for _, q := range [...]*queue{&s.frames, &s.timed, &s.fixed, &s.unscaled} {
		hashWaits(h, q.heap)
	}
	aw := make([]vm.CoHandle, 0, len(s.awaiting))
	for k := range s.awaiting {
		aw = append(aw, k)
	}
	slices.SortFunc(aw, func(a, b vm.CoHandle) int {
		if c := cmp.Compare(a.Index, b.Index); c != 0 {
			return c
		}
		return cmp.Compare(a.Gen, b.Gen)
	})
	for _, k := range aw {
		h.U32(k.Index)
		h.U32(k.Gen)
		hashWaits(h, s.awaiting[k])
	}
	hk := make([][2]uint64, 0, len(s.handles))
	for k := range s.handles {
		hk = append(hk, k)
	}
	slices.SortFunc(hk, func(a, b [2]uint64) int {
		if c := cmp.Compare(a[0], b[0]); c != 0 {
			return c
		}
		return cmp.Compare(a[1], b[1])
	})
	for _, k := range hk {
		h.U64(k[0])
		h.U64(k[1])
		hashWaits(h, s.handles[k])
	}
	hashItems(h, s.woken)
	hashItems(h, s.hwoken)
	h.Int(s.live)
	h.Int(s.waiters)
	h.U64(s.seq)
	h.F64(s.now)
	h.F64(s.real)
	h.U64(s.frame)
	h.U64(s.uframe)
	h.U64(s.fixedN)
}
