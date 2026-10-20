package spawn

import (
	"cmp"
	"slices"
	"strings"
	"unsafe"

	"aego/core/ids"
	"aego/core/rewind"
	"aego/scene"
)

var _ rewind.Keyframer = (*Service)(nil)

type memberKey struct {
	h      scene.NodeHandle
	prefab ids.AssetID
	nodes  []nodeSnap
	out    bool
}

type freeKey struct {
	prefab ids.AssetID
	n      int32
}

type pidKey struct {
	h   scene.NodeHandle
	pid ids.NodeID
}

type spawnKey struct {
	members []memberKey
	free    []freeKey
	freeH   []scene.NodeHandle
	parking []scene.NodeHandle
	pids    []pidKey
	order   []scene.NodeHandle
	root    scene.NodeHandle
	counter uint64
}

func (k *spawnKey) Size() int {
	hs := int(unsafe.Sizeof(scene.NodeHandle{}))
	return len(k.members)*int(unsafe.Sizeof(memberKey{})) + len(k.free)*int(unsafe.Sizeof(freeKey{})) +
		len(k.pids)*int(unsafe.Sizeof(pidKey{})) + (len(k.freeH)+len(k.parking)+len(k.order))*hs
}

func (s *Service) Capture(reuse rewind.Key) rewind.Key {
	k, _ := reuse.(*spawnKey)
	if k == nil {
		k = &spawnKey{}
	}
	k.members = k.members[:0]
	for h, m := range s.members {
		k.members = append(k.members, memberKey{h: h, prefab: m.prefab, nodes: m.nodes, out: m.out})
	}
	k.free, k.freeH = k.free[:0], k.freeH[:0]
	for p, list := range s.free {
		k.free = append(k.free, freeKey{prefab: p, n: int32(len(list))})
		k.freeH = append(k.freeH, list...)
	}
	k.parking = append(k.parking[:0], s.parking...)
	k.pids = k.pids[:0]
	for h, pid := range s.pids {
		k.pids = append(k.pids, pidKey{h: h, pid: pid})
	}
	k.order = append(k.order[:0], s.order...)
	k.root, k.counter = s.root, s.counter
	return k
}

func (s *Service) Restore(key rewind.Key) {
	k := key.(*spawnKey)
	clear(s.members)
	for _, m := range k.members {
		s.members[m.h] = &member{prefab: m.prefab, nodes: m.nodes, out: m.out}
	}
	clear(s.free)
	buf := slices.Clone(k.freeH)
	off := 0
	for _, f := range k.free {
		n := int(f.n)
		s.free[f.prefab] = buf[off : off+n : off+n]
		off += n
	}
	s.parking = append(s.parking[:0], k.parking...)
	clear(s.pids)
	clear(s.byPID)
	for _, p := range k.pids {
		s.pids[p.h] = p.pid
		s.byPID[p.pid] = p.h
	}
	s.order = append(s.order[:0], k.order...)
	s.root, s.counter = k.root, k.counter
}

func cmpHandle(a, b scene.NodeHandle) int {
	if c := cmp.Compare(a.Index, b.Index); c != 0 {
		return c
	}
	return cmp.Compare(a.Gen, b.Gen)
}

func hashHandles(h *rewind.Hasher, list []scene.NodeHandle) {
	h.Int(len(list))
	for _, n := range list {
		h.U32(n.Index)
		h.U32(n.Gen)
	}
}

func (s *Service) Hash(h *rewind.Hasher) {
	h.String("spawn")
	hs := make([]scene.NodeHandle, 0, len(s.members))
	for n := range s.members {
		hs = append(hs, n)
	}
	slices.SortFunc(hs, cmpHandle)
	for _, n := range hs {
		m := s.members[n]
		h.U32(n.Index)
		h.U32(n.Gen)
		h.String(m.prefab.String())
		h.Bool(m.out)
		h.Int(len(m.nodes))
	}
	ps := make([]ids.AssetID, 0, len(s.free))
	for p := range s.free {
		ps = append(ps, p)
	}
	slices.SortFunc(ps, func(a, b ids.AssetID) int { return strings.Compare(a.String(), b.String()) })
	for _, p := range ps {
		h.String(p.String())
		hashHandles(h, s.free[p])
	}
	hs = hs[:0]
	for n := range s.pids {
		hs = append(hs, n)
	}
	slices.SortFunc(hs, cmpHandle)
	for _, n := range hs {
		pid := s.pids[n]
		h.U32(n.Index)
		h.U32(n.Gen)
		h.U64(pid.Hi)
		h.U64(pid.Lo)
	}
	hashHandles(h, s.parking)
	hashHandles(h, s.order)
	h.U32(s.root.Index)
	h.U32(s.root.Gen)
	h.U64(s.counter)
}
