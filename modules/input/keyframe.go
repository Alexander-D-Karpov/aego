package input

import (
	"unsafe"

	"aego/core/rewind"
	raw "aego/platform/input"
)

var _ rewind.Keyframer = (*Service)(nil)

type inputKey struct {
	states  []state
	stack   []int
	order   []int
	inj     []injection
	devs    [MaxPlayers]uint8
	device  [MaxPlayers]int8
	padsOn  [raw.MaxPads]bool
	rebind  rebind
	clock   Clock
	frame   uint64
	next    int
	changed bool
	fixed   bool
}

func (k *inputKey) Size() int {
	return int(unsafe.Sizeof(*k)) + len(k.states)*int(unsafe.Sizeof(state{})) + 8*(len(k.stack)+len(k.order)) +
		len(k.inj)*int(unsafe.Sizeof(injection{}))
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*inputKey)
	if k == nil {
		k = &inputKey{}
	}
	k.states = append(k.states[:0], s.states...)
	k.stack = append(k.stack[:0], s.stack...)
	k.order = append(k.order[:0], s.order...)
	k.inj = append(k.inj[:0], s.inj...)
	k.devs, k.device, k.padsOn = s.devs, s.device, s.padsOn
	k.rebind, k.clock, k.frame = s.rebind, s.clock, s.frame
	k.changed, k.fixed = s.changed, s.fixed
	k.next = 0
	if s.play != nil {
		k.next = s.play.next
	}
	return k
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*inputKey)
	s.states = append(s.states[:0], k.states...)
	s.stack = append(s.stack[:0], k.stack...)
	s.order = append(s.order[:0], k.order...)
	s.inj = append(s.inj[:0], k.inj...)
	s.devs, s.device, s.padsOn = k.devs, k.device, k.padsOn
	s.rebind, s.clock, s.frame = k.rebind, k.clock, k.frame
	s.changed, s.fixed = k.changed, k.fixed
	s.rec = nil
	if s.play != nil {
		s.play.next = k.next
	}
}

func hashVec(h *rewind.Hasher, x, y float32) {
	h.F32(x)
	h.F32(y)
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String("input")
	h.Int(len(s.states))
	for i := range s.states {
		st := &s.states[i]
		hashVec(h, st.value.X, st.value.Y)
		hashVec(h, st.fValue.X, st.fValue.Y)
		hashVec(h, st.injVal.X, st.injVal.Y)
		for _, b := range [...]bool{st.down, st.pressed, st.released, st.fDown, st.fPressed, st.fReleased, st.consumed, st.fConsumed,
			st.active, st.release, st.rawPrev, st.latch, st.bufLive, st.inj} {
			h.Bool(b)
		}
		h.U32(uint32(st.pend[0]) | uint32(st.pend[1])<<8 | uint32(st.npend)<<16)
		h.I64(int64(st.blockedBy))
		h.I64(int64(st.via))
		h.I64(int64(st.why))
		h.U64(st.visit)
		h.F64(st.heldFor)
		h.F64(st.pressAt)
		h.F64(st.bufAt)
	}
	h.Int(len(s.stack))
	for _, c := range s.stack {
		h.Int(c)
	}
	for p := range s.devs {
		h.U32(uint32(s.devs[p]))
		h.I64(int64(s.device[p]))
	}
	for _, on := range s.padsOn {
		h.Bool(on)
	}
	h.Int(len(s.inj))
	for _, x := range s.inj {
		h.U64(x.tick)
		h.Int(x.player)
		h.Int(x.action)
		hashVec(h, x.value.X, x.value.Y)
	}
	h.U64(s.frame)
	h.Bool(s.changed)
	h.Bool(s.fixed)
}
