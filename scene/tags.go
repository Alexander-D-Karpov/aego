package scene

import (
	"reflect"
	"slices"

	"aego/core/strs"
	"aego/schema"
)

const TagsName = "engine.Tags"

type Tags struct {
	Tags []string `aego:"tags"`
}

var tagsType = reflect.TypeFor[Tags]()

type tagIndex struct {
	typ     schema.TypeID
	lists   map[strs.StringID][]NodeHandle
	byNode  map[NodeHandle][]strs.StringID
	pending map[NodeHandle]bool
	dirty   []NodeHandle
	scratch []strs.StringID
}

func ValidTag(tag string) bool {
	seg := 0
	for i := 0; i < len(tag); i++ {
		switch c := tag[i]; {
		case c == '.':
			if seg == 0 {
				return false
			}
			seg = 0
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			seg++
		default:
			return false
		}
	}
	return seg > 0
}

func (s *Scene) tagIdx() *tagIndex {
	if s.tags != nil || s.tagsChecked {
		return s.tags
	}
	s.tagsChecked = true
	td, ok := s.registry.ByGoType(tagsType)
	if !ok {
		return nil
	}
	x := &tagIndex{
		typ:     td.ID,
		lists:   map[strs.StringID][]NodeHandle{},
		byNode:  map[NodeHandle][]strs.StringID{},
		pending: map[NodeHandle]bool{},
	}
	s.tags = x
	st := storeOf[Tags](s)
	st.EachAll(func(h NodeHandle, _ *Tags) { x.mark(h) })
	for _, e := range st.pending {
		x.mark(e.node)
	}
	return x
}

func (x *tagIndex) mark(h NodeHandle) {
	if !x.pending[h] {
		x.pending[h] = true
		x.dirty = append(x.dirty, h)
	}
}

func (x *tagIndex) observe(e Event) {
	switch e.Kind {
	case EvComponentAdded, EvComponentRemoved, EvPropertyChanged:
		if e.Type == x.typ {
			x.mark(e.Node)
		}
	case EvNodeDestroyed:
		if _, ok := x.byNode[e.Node]; ok {
			x.mark(e.Node)
		}
	}
}

func (x *tagIndex) refresh(s *Scene) {
	if len(x.dirty) == 0 {
		return
	}
	for _, h := range x.dirty {
		x.reindex(s, h)
		delete(x.pending, h)
	}
	x.dirty = x.dirty[:0]
}

func dropNode(list []NodeHandle, h NodeHandle) []NodeHandle {
	if i := slices.Index(list, h); i >= 0 {
		return slices.Delete(list, i, i+1)
	}
	return list
}

func (x *tagIndex) reindex(s *Scene, h NodeHandle) {
	next := x.scratch[:0]
	if s.IsAlive(h) {
		if t, ok := Get[Tags](s, h); ok {
			for _, tag := range t.Tags {
				if !ValidTag(tag) {
					continue
				}
				for i := 0; i <= len(tag); i++ {
					if i < len(tag) && tag[i] != '.' {
						continue
					}
					if id := s.interner.Intern(tag[:i]); !slices.Contains(next, id) {
						next = append(next, id)
					}
				}
			}
		}
	}
	old := x.byNode[h]
	for _, id := range old {
		if !slices.Contains(next, id) {
			x.lists[id] = dropNode(x.lists[id], h)
		}
	}
	for _, id := range next {
		if !slices.Contains(old, id) {
			x.lists[id] = append(x.lists[id], h)
		}
	}
	x.scratch = next
	if len(next) == 0 {
		delete(x.byNode, h)
		return
	}
	x.byNode[h] = append(old[:0], next...)
}

func (s *Scene) HasTag(h NodeHandle, tag string) bool {
	x := s.tagIdx()
	if x == nil {
		return false
	}
	x.refresh(s)
	id, ok := s.interner.Lookup(tag)
	return ok && slices.Contains(x.byNode[h], id)
}

func (s *Scene) AddTag(h NodeHandle, tag string) bool {
	x := s.tagIdx()
	if x == nil || !ValidTag(tag) || !s.IsAlive(h) {
		return false
	}
	t := Add[Tags](s, h)
	if t == nil {
		return false
	}
	if !slices.Contains(t.Tags, tag) {
		t.Tags = append(t.Tags, tag)
		x.mark(h)
		s.touch()
	}
	return true
}

func (s *Scene) RemoveTag(h NodeHandle, tag string) bool {
	x := s.tagIdx()
	if x == nil {
		return false
	}
	t, ok := Get[Tags](s, h)
	if !ok {
		return false
	}
	i := slices.Index(t.Tags, tag)
	if i < 0 {
		return false
	}
	t.Tags = slices.Delete(t.Tags, i, i+1)
	x.mark(h)
	s.touch()
	return true
}

func (s *Scene) TagsChanged(h NodeHandle) {
	if x := s.tagIdx(); x != nil {
		x.mark(h)
	}
}

func (s *Scene) withTag(tag string, out []NodeHandle, all bool) int {
	x := s.tagIdx()
	if x == nil || len(out) == 0 {
		return 0
	}
	x.refresh(s)
	id, ok := s.interner.Lookup(tag)
	if !ok {
		return 0
	}
	n := 0
	for _, h := range x.lists[id] {
		if n == len(out) {
			break
		}
		nd, ok := s.node(h)
		if !ok || nd.has(flagPendingDestroy) || (!all && !nd.has(flagEffective)) {
			continue
		}
		out[n] = h
		n++
	}
	return n
}

func (s *Scene) WithTag(tag string, out []NodeHandle) int {
	return s.withTag(tag, out, false)
}

func (s *Scene) WithTagAll(tag string, out []NodeHandle) int {
	return s.withTag(tag, out, true)
}
