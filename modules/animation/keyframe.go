package animation

import (
	"unsafe"

	"aego/anim"
	"aego/core/rewind"
	"aego/scene"
)

var _ rewind.Keyframer = (*Service)(nil)

const (
	keyBase uint8 = 1 << iota
	keyLive
)

type animKey struct {
	nodes  []scene.NodeHandle
	count  []int32
	anodes []scene.NodeHandle
	acount []int32
	vals   []anim.Value
	flags  []uint8
}

func (k *animKey) Size() int {
	return (len(k.nodes)+len(k.anodes))*int(unsafe.Sizeof(scene.NodeHandle{})) + 4*(len(k.count)+len(k.acount)) +
		len(k.vals)*int(unsafe.Sizeof(anim.Value{})) + len(k.flags)
}

func (k *animKey) put(binds []binding) {
	for i := range binds {
		b := &binds[i]
		var f uint8
		if b.hasBase {
			f |= keyBase
		}
		if b.live {
			f |= keyLive
		}
		k.vals = append(k.vals, b.base)
		k.flags = append(k.flags, f)
	}
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*animKey)
	if k == nil {
		k = &animKey{}
	}
	k.nodes, k.count, k.anodes, k.acount = k.nodes[:0], k.count[:0], k.anodes[:0], k.acount[:0]
	k.vals, k.flags = k.vals[:0], k.flags[:0]
	s.key = k
	scene.Store[AnimationPlayer](s.scene).EachAll(s.captureFn)
	scene.Store[Animator](s.scene).EachAll(s.acaptureFn)
	s.key = nil
	return k
}

func (s *Service) captureOne(h scene.NodeHandle, c *AnimationPlayer) {
	k := s.key
	k.nodes = append(k.nodes, h)
	p := s.lookup(h, c)
	if p == nil {
		k.count = append(k.count, -1)
		return
	}
	k.count = append(k.count, int32(len(p.binds)))
	k.put(p.binds)
}

func (s *Service) acaptureOne(h scene.NodeHandle, a *Animator) {
	k := s.key
	k.anodes = append(k.anodes, h)
	r := s.alookup(h, a)
	if r == nil {
		k.acount = append(k.acount, -1)
		return
	}
	k.acount = append(k.acount, int32(len(r.binds)))
	k.put(r.binds)
}

func (s *Service) restoreBinds(binds []binding, ok bool, n int, k *animKey, off int) int {
	if n < 0 || !ok || len(binds) != n {
		s.dirty = true
		return off + max(n, 0)
	}
	for j := range binds {
		b := &binds[j]
		f := k.flags[off+j]
		b.base, b.hasBase, b.live, b.wrote = k.vals[off+j], f&keyBase != 0, f&keyLive != 0, false
	}
	return off + n
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*animKey)
	off := 0
	for i, h := range k.nodes {
		p := s.players[h]
		var binds []binding
		if p != nil {
			binds = p.binds
		}
		off = s.restoreBinds(binds, p != nil, int(k.count[i]), k, off)
	}
	for i, h := range k.anodes {
		r := s.anims[h]
		var binds []binding
		if r != nil {
			binds = r.binds
		}
		off = s.restoreBinds(binds, r != nil, int(k.acount[i]), k, off)
	}
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String(ServiceID)
	s.hh = h
	scene.Store[AnimationPlayer](s.scene).EachAll(s.hashFn)
	scene.Store[Animator](s.scene).EachAll(s.ahashFn)
	s.hh = nil
}

func (s *Service) hashOne(n scene.NodeHandle, c *AnimationPlayer) {
	h, st := s.hh, &c.st
	h.U32(n.Index)
	h.U32(n.Gen)
	if c.TimeDomain == DomainGame {
		h.F64(st.time)
	}
	h.F32(st.seek)
	h.U32(uint32(st.cmd))
	for _, b := range [...]bool{st.playing, st.paused, st.first, st.started, st.auto} {
		h.Bool(b)
	}
}

func (s *Service) ahashOne(n scene.NodeHandle, a *Animator) {
	h, st := s.hh, &a.st
	h.U32(n.Index)
	h.U32(n.Gen)
	if a.TimeDomain == DomainGame {
		m := &st.m
		h.I64(int64(m.State))
		h.F64(m.Time)
		h.F64(m.Last)
		h.I64(int64(m.Prev))
		h.F64(m.PrevTime)
		h.F64(m.PrevLast)
		h.F32(m.BlendT)
		h.F32(m.BlendDur)
		h.Bool(m.Blend)
		h.Bool(m.Started)
	}
	for _, v := range st.params {
		h.F64(v)
	}
	h.U64(st.ctrl.Hi)
	h.U64(st.ctrl.Lo)
	h.I64(int64(st.target))
	h.F32(st.fade)
	h.U32(uint32(st.cmd))
	h.Bool(st.on)
	h.Bool(st.auto)
}
