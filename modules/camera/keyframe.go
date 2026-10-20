package camera

import (
	"unsafe"

	"aego/core/rewind"
	"aego/scene"
)

var _ rewind.Keyframer = (*Service)(nil)

type camKey struct {
	outs    []output
	follows []follow
	zones   []zone
	seq     uint64
	next    uint64
	ticks   uint64
}

func (k *camKey) Size() int {
	return len(k.outs)*int(unsafe.Sizeof(output{})) + len(k.follows)*int(unsafe.Sizeof(follow{})) + len(k.zones)*int(unsafe.Sizeof(zone{}))
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*camKey)
	if k == nil {
		k = &camKey{}
	}
	k.outs = k.outs[:0]
	for _, o := range s.outputs {
		c := *o
		c.vcs = nil
		k.outs = append(k.outs, c)
	}
	k.follows = append(k.follows[:0], s.follows...)
	k.zones = append(k.zones[:0], s.zones...)
	k.seq, k.next, k.ticks = s.seq, s.nextKey, s.ticks
	return k
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*camKey)
	outs := make([]*output, len(k.outs))
	for i, c := range k.outs {
		if cur := s.find(c.name); cur != nil {
			c.vcs = cur.vcs
		}
		if !c.game {
			c.blend.on = false
		}
		o := c
		outs[i] = &o
	}
	s.outputs = outs
	s.follows = append(s.follows[:0], k.follows...)
	s.zones = append(s.zones[:0], k.zones...)
	s.seq, s.nextKey, s.ticks = k.seq, k.next, k.ticks
}

func hashNode(h *rewind.Hasher, n scene.NodeHandle) {
	h.U32(n.Index)
	h.U32(n.Gen)
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String("camera")
	h.Int(len(s.outputs))
	for _, o := range s.outputs {
		h.String(o.name)
		hashNode(h, o.active)
		hashNode(h, o.zone)
		hashNode(h, o.forced)
		h.Bool(o.game)
		if !o.game {
			continue
		}
		h.Bool(o.blend.on)
		h.F32(o.blend.t)
		h.F32(o.blend.dur)
		h.F32(o.trauma)
		h.F32(o.noiseT)
	}
	h.Int(len(s.zones))
	for _, z := range s.zones {
		hashNode(h, z.node)
		h.Bool(z.inside)
		h.U64(z.seq)
	}
	h.U64(s.seq)
}
