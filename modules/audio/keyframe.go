package audio

import (
	"unsafe"

	"aego/core/rewind"
	"aego/core/rng"
)

var _ rewind.Keyframer = (*Service)(nil)

type audioKey struct {
	cool  []coolEntry
	zones []zoneState
	rng   rng.Source
	time  float64
	seq   uint64
	ready bool
}

func (k *audioKey) Size() int {
	return int(unsafe.Sizeof(*k)) + len(k.cool)*int(unsafe.Sizeof(coolEntry{})) + len(k.zones)*int(unsafe.Sizeof(zoneState{}))
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*audioKey)
	if k == nil {
		k = &audioKey{}
	}
	k.cool = append(k.cool[:0], s.cool...)
	k.zones = append(k.zones[:0], s.zones...)
	k.rng = *s.rng
	k.time, k.seq, k.ready = s.time, s.seq, s.ready
	return k
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*audioKey)
	s.drop(true, 0)
	s.cool = append(s.cool[:0], k.cool...)
	s.zones = append(s.zones[:0], k.zones...)
	*s.rng = k.rng
	s.time, s.seq, s.ready = k.time, k.seq, k.ready
	for b := range s.worldG {
		s.worldG[b] = -1
	}
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String("audio")
	h.Bool(s.ready)
	h.Int(len(s.cool))
	for _, c := range s.cool {
		h.U64(c.id.Hi)
		h.U64(c.id.Lo)
		h.F64(c.until)
	}
	h.Int(len(s.zones))
	for _, z := range s.zones {
		h.U32(z.node.Index)
		h.U32(z.node.Gen)
		h.Bool(z.inside)
		h.U64(z.seq)
	}
	h.U64(s.seq)
}
