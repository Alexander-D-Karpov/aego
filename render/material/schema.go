package material

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"sync"

	"aego/core/ids"
	"aego/render/shader"
)

type Schema struct {
	Params   []shader.Param
	Size     int
	Lanes    int
	Textures []int
	index    map[string]int
	lanes    [16]float32
	hash     uint64
}

func NewSchema(params []shader.Param, size, lanes int) *Schema {
	s := &Schema{Params: params, Size: size, Lanes: lanes, index: make(map[string]int, len(params))}
	h := fnv.New64a()
	var word [8]byte
	put := func(n int) { binary.LittleEndian.PutUint64(word[:], uint64(n)); h.Write(word[:]) }
	put(size)
	put(lanes)
	put(len(params))
	for i := range params {
		p := &params[i]
		s.index[p.Name] = i
		for _, former := range p.Former {
			if _, exists := s.index[former]; !exists {
				s.index[former] = i
			}
		}
		h.Write([]byte(p.Name))
		h.Write([]byte{0, byte(p.Kind), flag(p.Instance), flag(p.Normal), flag(p.Emission)})
		put(p.Offset)
		put(p.Lane)
		put(p.Comp)
		put(p.Slot)
		switch {
		case p.Kind == shader.KindTexture:
			s.Textures = append(s.Textures, i)
		case p.Instance:
			w := defaultWords(p)
			for c := 0; c < p.Kind.Lanes(); c++ {
				s.lanes[p.Lane*4+p.Comp+c] = math.Float32frombits(w[c])
			}
		}
	}
	s.hash = h.Sum64()
	return s
}

func flag(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func (s *Schema) Param(name string) (*shader.Param, bool) {
	i, ok := s.index[name]
	if !ok {
		return nil, false
	}
	return &s.Params[i], true
}

func (s *Schema) Hash() uint64 {
	return s.hash
}

func (s *Schema) DefaultLanes() *[16]float32 {
	return &s.lanes
}

type Schemas struct {
	mu   sync.RWMutex
	byID map[ids.AssetID]*Schema
}

func (r *Schemas) Set(id ids.AssetID, s *Schema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = map[ids.AssetID]*Schema{}
	}
	r.byID[id] = s
}

func (r *Schemas) Get(id ids.AssetID) (*Schema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byID[id]
	return s, ok
}

func (r *Schemas) Delete(id ids.AssetID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}
