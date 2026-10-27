package material

import (
	"hash/fnv"
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
	for i := range params {
		p := &params[i]
		s.index[p.Name] = i
		h.Write([]byte(p.Name))
		h.Write([]byte{byte(p.Kind), flag(p.Instance), byte(p.Offset), byte(p.Lane<<4 | p.Comp), byte(p.Slot)})
		switch {
		case p.Kind == shader.KindTexture:
			s.Textures = append(s.Textures, i)
		case p.Instance:
			copy(s.lanes[p.Lane*4+p.Comp:], p.Default[:p.Kind.Lanes()])
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
