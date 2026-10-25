package tween

import (
	"unsafe"

	"aego/core/curve"
	"aego/core/rewind"
)

var _ rewind.Keyframer = (*Service)(nil)

type tweenKey struct {
	tweens  []slot[tween]
	tfree   []uint32
	ckeys   []curve.Key
	ckn     []int32
	timers  []slot[timer]
	mfree   []uint32
	actions []slot[action]
	afree   []uint32
	steps   []actStep
	stn     []int32
	tn      int
	mn      int
	an      int
}

func (k *tweenKey) Size() int {
	return len(k.tweens)*int(unsafe.Sizeof(slot[tween]{})) + len(k.timers)*int(unsafe.Sizeof(slot[timer]{})) +
		len(k.actions)*int(unsafe.Sizeof(slot[action]{})) + len(k.ckeys)*int(unsafe.Sizeof(curve.Key{})) +
		len(k.steps)*int(unsafe.Sizeof(actStep{})) + 4*(len(k.tfree)+len(k.mfree)+len(k.afree)+len(k.ckn)+len(k.stn))
}

func fitSlots[T any](s []slot[T], n int) []slot[T] {
	if cap(s) < n {
		out := make([]slot[T], n)
		copy(out, s[:cap(s)])
		return out
	}
	return s[:n]
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*tweenKey)
	if k == nil {
		k = &tweenKey{}
	}
	k.tweens = append(k.tweens[:0], s.tweens.s...)
	k.ckeys, k.ckn = k.ckeys[:0], k.ckn[:0]
	for i := range k.tweens {
		cv := &k.tweens[i].v.cv
		k.ckn = append(k.ckn, int32(len(cv.Keys)))
		k.ckeys = append(k.ckeys, cv.Keys...)
		cv.Keys = nil
	}
	k.tfree, k.tn = append(k.tfree[:0], s.tweens.free...), s.tweens.n
	k.timers = append(k.timers[:0], s.timers.s...)
	k.mfree, k.mn = append(k.mfree[:0], s.timers.free...), s.timers.n
	k.actions = append(k.actions[:0], s.actions.s...)
	k.steps, k.stn = k.steps[:0], k.stn[:0]
	for i := range k.actions {
		a := &k.actions[i].v
		k.stn = append(k.stn, int32(len(a.steps)))
		k.steps = append(k.steps, a.steps...)
		a.steps = nil
	}
	k.afree, k.an = append(k.afree[:0], s.actions.free...), s.actions.n
	return k
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*tweenKey)
	ts := fitSlots(s.tweens.s, len(k.tweens))
	off := 0
	for i := range k.tweens {
		prev := ts[i].v.cv.Keys
		ts[i] = k.tweens[i]
		n := int(k.ckn[i])
		ts[i].v.cv.Keys = append(prev[:0], k.ckeys[off:off+n]...)
		off += n
	}
	s.tweens = pool[tween]{s: ts, free: append(s.tweens.free[:0], k.tfree...), n: k.tn}
	s.timers = pool[timer]{s: append(s.timers.s[:0], k.timers...), free: append(s.timers.free[:0], k.mfree...), n: k.mn}
	as := fitSlots(s.actions.s, len(k.actions))
	off = 0
	for i := range k.actions {
		prev := as[i].v.steps
		as[i] = k.actions[i]
		n := int(k.stn[i])
		as[i].v.steps = append(prev[:0], k.steps[off:off+n]...)
		off += n
	}
	s.actions = pool[action]{s: as, free: append(s.actions.free[:0], k.afree...), n: k.an}
}

func hashFree(h *rewind.Hasher, free []uint32, n int) {
	h.Int(len(free))
	for _, f := range free {
		h.U32(f)
	}
	h.Int(n)
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String("tween")
	h.Int(len(s.tweens.s))
	for i := range s.tweens.s {
		e := &s.tweens.s[i]
		h.U32(e.gen)
		h.Bool(e.live)
		if !e.live {
			continue
		}
		t := &e.v
		h.U32(t.node.Index)
		h.U32(t.node.Gen)
		t.tgt.HashTo(h)
		for j := range 4 {
			h.F32(t.from[j])
			h.F32(t.to[j])
		}
		h.F32(t.dur)
		h.F32(t.t)
		h.F32(t.delay)
		h.I64(int64(t.legs))
		h.U64(uint64(t.ease))
		h.Int(len(t.cv.Keys))
		for _, b := range [...]bool{t.hasCurve, t.ping, t.rev, t.started, t.unscaled, t.held, t.moved} {
			h.Bool(b)
		}
		h.U64(t.act.Key())
	}
	hashFree(h, s.tweens.free, s.tweens.n)
	h.Int(len(s.timers.s))
	for i := range s.timers.s {
		e := &s.timers.s[i]
		h.U32(e.gen)
		h.Bool(e.live)
		if !e.live {
			continue
		}
		t := &e.v
		h.F32(t.dur)
		h.F32(t.left)
		h.I64(int64(t.legs))
		h.I64(int64(t.repeat))
		h.U32(t.count)
		h.U32(t.owner.Index)
		h.U32(t.owner.Gen)
		for _, b := range [...]bool{t.paused, t.unscaled, t.keep, t.done, t.held, t.moved} {
			h.Bool(b)
		}
		h.U64(t.act.Key())
	}
	hashFree(h, s.timers.free, s.timers.n)
	h.Int(len(s.actions.s))
	for i := range s.actions.s {
		e := &s.actions.s[i]
		h.U32(e.gen)
		h.Bool(e.live)
		if !e.live {
			continue
		}
		a := &e.v
		h.Bool(a.parallel)
		h.Bool(a.started)
		h.Bool(a.held)
		h.Int(a.cur)
		h.Int(a.left)
		h.U32(a.owner.Index)
		h.U32(a.owner.Gen)
		h.U64(a.parent.Key())
		h.Int(len(a.steps))
		for _, st := range a.steps {
			h.U64(uint64(st.kind))
			h.U64(st.h.Key())
			h.Bool(st.done)
		}
	}
	hashFree(h, s.actions.free, s.actions.n)
}
