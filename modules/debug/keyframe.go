package debug

import (
	"cmp"
	"slices"
	"unsafe"

	"aego/contracts/debugdraw"
	"aego/core/mathx"
	"aego/core/rewind"
	"aego/script/vm"
)

var _ rewind.Keyframer = (*Service)(nil)

type penKey struct {
	owner vm.InstanceID
	p     pen
}

type debugKey struct {
	prims   []prim
	pts     []mathx.Vec2
	chans   []channel
	pens    []penKey
	next    debugdraw.Handle
	dropped uint64
}

func (k *debugKey) Size() int {
	return len(k.prims)*int(unsafe.Sizeof(prim{})) + len(k.pts)*int(unsafe.Sizeof(mathx.Vec2{})) +
		len(k.chans)*int(unsafe.Sizeof(channel{})) + len(k.pens)*int(unsafe.Sizeof(penKey{}))
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*debugKey)
	if k == nil {
		k = &debugKey{}
	}
	k.prims = append(k.prims[:0], s.prims...)
	k.pts = append(k.pts[:0], s.pts...)
	k.chans = append(k.chans[:0], s.chans...)
	k.pens = k.pens[:0]
	for o, p := range s.pens {
		k.pens = append(k.pens, penKey{owner: o, p: p})
	}
	k.next, k.dropped = s.next, s.dropped
	return k
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*debugKey)
	s.prims = append(s.prims[:0], k.prims...)
	s.pts = append(s.pts[:0], k.pts...)
	views := make([]channel, 0, len(s.chans))
	for _, c := range s.chans {
		views = append(views, channel{name: c.name, state: c.state})
	}
	s.chans = append(s.chans[:0], k.chans...)
	clear(s.byName)
	for i, c := range s.chans {
		s.byName[c.name] = uint16(i)
	}
	for _, v := range views {
		if i, ok := s.byName[v.name]; ok {
			s.chans[i].state = v.state
			continue
		}
		s.byName[v.name] = uint16(len(s.chans))
		s.chans = append(s.chans, v)
	}
	if s.pens == nil {
		s.pens = map[vm.InstanceID]pen{}
	}
	clear(s.pens)
	for _, p := range k.pens {
		s.pens[p.owner] = p.p
	}
	s.next, s.dropped = k.next, k.dropped
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String("debugdraw")
	h.U64(uint64(s.next))
	h.U64(s.dropped)
	h.Int(len(s.prims))
	for i := range s.prims {
		p := &s.prims[i]
		h.U64(uint64(p.kind))
		h.Bool(p.gone)
		h.U64(uint64(p.id))
		h.String(s.chans[p.ch].name)
		h.F32(p.a.X)
		h.F32(p.a.Y)
		h.F32(p.b.X)
		h.F32(p.b.Y)
		h.F32(p.r)
		h.F32(p.left)
		h.U64(uint64(p.life))
		h.U32(p.owner.Index)
		h.U32(p.owner.Gen)
		h.String(p.text)
	}
	h.Int(len(s.pts))
	for _, v := range s.pts {
		h.F32(v.X)
		h.F32(v.Y)
	}
	for _, c := range s.chans {
		if c.count > 0 {
			h.String(c.name)
			h.Int(c.count)
		}
	}
	owners := make([]vm.InstanceID, 0, len(s.pens))
	for o := range s.pens {
		owners = append(owners, o)
	}
	slices.SortFunc(owners, func(a, b vm.InstanceID) int { return cmp.Compare(a, b) })
	for _, o := range owners {
		p := s.pens[o]
		h.U32(uint32(o))
		h.String(p.ch)
		h.U64(uint64(p.life.Kind))
		h.F32(p.life.N)
	}
}
