package scene

import (
	"aego/core/ids"
	"aego/schema"
)

type EventKind uint8

const (
	EvNodeCreated EventKind = iota
	EvNodeDestroyed
	EvNodeReparented
	EvNodeRenamed
	EvComponentAdded
	EvComponentRemoved
	EvPropertyChanged
	EvHierarchyChanged
)

var eventNames = [...]string{
	"NodeCreated", "NodeDestroyed", "NodeReparented", "NodeRenamed",
	"ComponentAdded", "ComponentRemoved", "PropertyChanged", "HierarchyChanged",
}

func (k EventKind) String() string {
	if int(k) < len(eventNames) {
		return eventNames[k]
	}
	return "Unknown"
}

type Event struct {
	Kind  EventKind
	Node  NodeHandle
	ID    ids.NodeID
	Type  schema.TypeID
	Field schema.FieldID
}

type subscriber struct {
	id uint64
	fn func(Event)
}

type Events struct {
	subs   []subscriber
	nextID uint64
	depth  int
}

func (e *Events) Subscribe(fn func(Event)) func() {
	e.nextID++
	id := e.nextID
	e.subs = append(e.subs, subscriber{id: id, fn: fn})
	return func() {
		for i := range e.subs {
			if e.subs[i].id == id {
				e.subs = append(e.subs[:i], e.subs[i+1:]...)
				return
			}
		}
	}
}

func (e *Events) dispatch(ev Event) {
	if len(e.subs) == 0 || e.depth > 4 {
		return
	}
	e.depth++
	for i := range e.subs {
		e.subs[i].fn(ev)
	}
	e.depth--
}

func (e *Events) Len() int {
	return len(e.subs)
}
