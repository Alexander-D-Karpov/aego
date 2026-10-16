package selection

import "aego/core/ids"

type DocID uint32

type Item struct {
	Doc  DocID
	Node ids.NodeID
}

type subscriber struct {
	id uint64
	fn func()
}

type Service struct {
	items   []Item
	rev     uint64
	subs    []subscriber
	nextSub uint64
}

func New() *Service {
	return &Service{}
}

func (s *Service) Revision() uint64 {
	return s.rev
}

func (s *Service) changed() {
	s.rev++
	for i := range s.subs {
		s.subs[i].fn()
	}
}

func (s *Service) index(it Item) int {
	for i := range s.items {
		if s.items[i] == it {
			return i
		}
	}
	return -1
}

func (s *Service) Set(items ...Item) {
	s.items = s.items[:0]
	for _, it := range items {
		if s.index(it) < 0 {
			s.items = append(s.items, it)
		}
	}
	s.changed()
}

func (s *Service) Add(items ...Item) {
	for _, it := range items {
		if i := s.index(it); i >= 0 {
			s.items = append(s.items[:i], s.items[i+1:]...)
		}
		s.items = append(s.items, it)
	}
	s.changed()
}

func (s *Service) Remove(items ...Item) {
	for _, it := range items {
		if i := s.index(it); i >= 0 {
			s.items = append(s.items[:i], s.items[i+1:]...)
		}
	}
	s.changed()
}

func (s *Service) Toggle(it Item) {
	if i := s.index(it); i >= 0 {
		s.items = append(s.items[:i], s.items[i+1:]...)
	} else {
		s.items = append(s.items, it)
	}
	s.changed()
}

func (s *Service) Clear() {
	if len(s.items) == 0 {
		return
	}
	s.items = s.items[:0]
	s.changed()
}

func (s *Service) Items() []Item {
	return s.items
}

func (s *Service) Len() int {
	return len(s.items)
}

func (s *Service) Active() (Item, bool) {
	if n := len(s.items); n > 0 {
		return s.items[n-1], true
	}
	return Item{}, false
}

func (s *Service) Contains(it Item) bool {
	return s.index(it) >= 0
}

func (s *Service) Nodes(doc DocID, dst []ids.NodeID) []ids.NodeID {
	dst = dst[:0]
	for _, it := range s.items {
		if it.Doc == doc {
			dst = append(dst, it.Node)
		}
	}
	return dst
}

func (s *Service) Prune(doc DocID, alive func(ids.NodeID) bool) {
	n := 0
	for _, it := range s.items {
		if it.Doc != doc || alive(it.Node) {
			s.items[n] = it
			n++
		}
	}
	if n != len(s.items) {
		s.items = s.items[:n]
		s.changed()
	}
}

func (s *Service) DropDoc(doc DocID) {
	s.Prune(doc, func(ids.NodeID) bool { return false })
}

func (s *Service) Subscribe(fn func()) func() {
	s.nextSub++
	id := s.nextSub
	s.subs = append(s.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range s.subs {
			if s.subs[i].id == id {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				return
			}
		}
	}
}
