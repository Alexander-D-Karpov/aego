package savegame

import (
	"unsafe"

	"aego/core/rewind"
	"aego/format/aetext"
	"aego/save"
	"aego/scene"
)

var _ rewind.Provider = (*Service)(nil)

type saveKey struct {
	reqs     []request
	readers  map[scene.NodeHandle]*save.Reader
	global   []aetext.Field
	unknown  []save.Section
	since    float64
	playtime float64
	autosave float64
	applied  bool
}

func (k *saveKey) Size() int {
	return int(unsafe.Sizeof(*k)) + len(k.reqs)*int(unsafe.Sizeof(request{})) + 64*len(k.global)
}

type saveKeys struct {
	s *Service
}

func (s *Service) Keyframer() rewind.Keyframer {
	return saveKeys{s: s}
}

func (k saveKeys) Capture(reuse rewind.Key) rewind.Key {
	s := k.s
	key, _ := reuse.(*saveKey)
	if key == nil {
		key = &saveKey{readers: map[scene.NodeHandle]*save.Reader{}}
	}
	key.reqs = append(key.reqs[:0], s.reqs...)
	clear(key.readers)
	for h, r := range s.readers {
		key.readers[h] = r
	}
	key.global = cloneFields(s.m.global.Fields)
	key.unknown = append(key.unknown[:0], s.m.unknown...)
	key.since, key.applied = s.since, s.applied
	key.playtime, key.autosave = s.m.playtime, s.m.autosave
	return key
}

func (k saveKeys) Restore(r rewind.Key) {
	s, key := k.s, r.(*saveKey)
	s.reqs = append(s.reqs[:0], key.reqs...)
	clear(s.readers)
	for h, rd := range key.readers {
		s.readers[h] = rd
	}
	s.m.global.Fields = cloneFields(key.global)
	s.m.unknown = append(s.m.unknown[:0], key.unknown...)
	s.since, s.applied = key.since, key.applied
	s.m.playtime, s.m.autosave = key.playtime, key.autosave
}

func (k saveKeys) Hash(h *rewind.Hasher) {
	s := k.s
	h.String("save")
	h.F64(s.m.playtime)
	h.F64(s.m.autosave)
	h.F64(s.since)
	h.Bool(s.applied)
	h.Int(len(s.reqs))
	for _, r := range s.reqs {
		h.U64(uint64(r.kind))
		h.String(r.slot)
	}
	h.Bytes(aetext.Write(&aetext.Node{Head: "global", Fields: s.m.global.Fields}))
	h.Int(len(s.m.unknown))
}
